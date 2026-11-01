import importlib.util
import hashlib
import json
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("wybo_deploy", ROOT / "deploy/render.py")
deploy = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(deploy)


class NativeDeployTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="wybo-deploy-test-")
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)
        self.state_file = self.path / "deployment.json"
        self.state = deploy.init_state(self.state_file, "shop.example.com", "ops@example.com", "admin")

    def test_repeated_init_preserves_all_credentials(self):
        before = self.state_file.read_bytes()
        again = deploy.init_state(self.state_file, "shop.example.com", "ops@example.com", "admin")
        self.assertEqual(self.state, again)
        self.assertEqual(before, self.state_file.read_bytes())
        other = deploy.init_state(self.path / "other.json", "shop.example.com", "ops@example.com", "admin")
        for key in ("db_password", "jwt_secret", "admin_password"):
            self.assertNotEqual(again[key], other[key])
        with self.assertRaises(ValueError):
            deploy.init_state(self.state_file, "other.example.com", "ops@example.com", "admin")
        self.assertEqual(before, self.state_file.read_bytes())

    def test_rejects_injection_and_invalid_inputs(self):
        for domain in ("shop.example.com; id", "https://shop.example.com", "example.com\nserver{}", "*.example.com", "127.0.0.1", "localhost", "a..com", "a" * 64 + ".com", "x.local", "x.123", "../x.com", "UPPER.example.com"):
            with self.subTest(domain=domain), self.assertRaises(ValueError):
                deploy.validate(domain, "ops@example.com", "admin")
        for email in ("ops@example.com\nROOT=1", "not-an-email", "ops@example.com;id"):
            with self.subTest(email=email), self.assertRaises(ValueError):
                deploy.validate("shop.example.com", email, "admin")
        for admin in ("a", "admin\nPASSWORD=x", "a;id", "a" * 33):
            with self.subTest(admin=admin), self.assertRaises(ValueError):
                deploy.validate("shop.example.com", "ops@example.com", admin)

    def test_secrets_permissions_and_corrupted_state(self):
        self.assertEqual(stat.S_IMODE(self.state_file.stat().st_mode), 0o600)
        self.state_file.chmod(0o644)
        with self.assertRaises(ValueError):
            deploy.load_state(self.state_file)
        self.state_file.chmod(0o600)
        link = self.path / "link.json"
        link.symlink_to(self.state_file)
        with self.assertRaises(ValueError):
            deploy.load_state(link)
        bad = dict(self.state, db_password="' OR true --")
        self.state_file.write_text(json.dumps(bad))
        with self.assertRaises(ValueError):
            deploy.load_state(self.state_file)

    def test_render_loopback_db_tls_and_public_files_have_no_secrets(self):
        out = self.path / "rendered"
        deploy.render(self.state, out)
        config = json.loads((out / "config.yaml").read_text())
        self.assertEqual(config["server"]["addr"], "127.0.0.1:8080")
        self.assertEqual(config["server"]["trusted_proxies"], ["127.0.0.1", "::1"])
        self.assertEqual(config["db"]["driver"], "postgres")
        self.assertEqual(config["pay"]["notify_base"], "https://shop.example.com/api/v1/pay/notify")
        env = (out / "wybo.env").read_text()
        self.assertIn('JWT_SECRET="' + self.state["jwt_secret"] + '"', env)
        self.assertIn("dbname=wybo sslmode=disable", env)
        self.assertIn("user=wybo", env)
        self.assertNotIn("2FA", env)
        for filename in ("wybo.env", "credentials.txt"):
            self.assertEqual(stat.S_IMODE((out / filename).stat().st_mode), 0o600)
        for filename in ("config.yaml", "nginx-http.conf", "nginx-https.conf", "wybo.service"):
            text = (out / filename).read_text()
            for key in ("db_password", "jwt_secret", "admin_password"):
                self.assertNotIn(self.state[key], text)
        nginx = (out / "nginx-https.conf").read_text()
        self.assertIn("location ^~ /api/", nginx)
        self.assertIn("proxy_set_header X-Forwarded-For $remote_addr", nginx)
        self.assertIn("try_files $uri $uri/ /index.html", nginx)
        self.assertIn("/etc/letsencrypt/live/shop.example.com/fullchain.pem", nginx)
        self.assertNotIn("access_log /", nginx)

    def test_site_bootstrap_never_replaces_existing_settings_or_touches_2fa(self):
        sql = deploy.site_sql(self.state)
        self.assertEqual(sql.count("ON CONFLICT (key) DO NOTHING"), 3)
        self.assertNotIn("UPDATE", sql)
        self.assertNotIn("admins", sql)
        self.assertNotIn("totp", sql.lower())
        role_sql = deploy.database_sql(self.state)
        self.assertIn("NOSUPERUSER NOCREATEDB NOCREATEROLE", role_sql)
        self.assertNotIn("ALTER ROLE", role_sql)
        self.assertNotIn("DROP", role_sql)

    def test_toolchain_selects_supported_latest_patch_and_rejects_prereleases(self):
        go_mod = self.path / "go.mod"
        go_mod.write_text("module test\n\ngo 1.25.2\n")
        go_data = [{"version": v, "stable": s} for v, s in (("go1.26.0", True), ("go1.25.1", True), ("go1.25.12", True), ("go1.25.3", True), ("go1.25.13rc1", False))]
        self.assertEqual(deploy.versions("go", go_data, go_mod), "go1.25.12")
        with self.assertRaises(ValueError):
            deploy.versions("go", [{"version": "go1.25.1", "stable": True}], go_mod)
        node_data = [{"version": v, "lts": lts} for v, lts in (("v24.0.0", "Krypton"), ("v22.9.0", "Jod"), ("v22.15.1", "Jod"), ("v22.99.0-rc.1", False))]
        self.assertEqual(deploy.versions("node", node_data), "v22.15.1")

    def test_cli_dry_run_creates_only_preview_and_does_not_print_secrets(self):
        out = self.path / "preview"
        command = ["bash", str(ROOT / "deploy.sh"), "--dry-run", "--domain", "shop.example.com", "--email", "ops@example.com", "--output-dir", str(out)]
        first = subprocess.run(command, cwd=ROOT, text=True, capture_output=True, check=True)
        sample = deploy.load_state(out / "deployment.json")
        for key in ("db_password", "admin_password", "jwt_secret"):
            self.assertNotIn(sample[key], first.stdout + first.stderr)
        self.assertTrue((out / "wybo-backup").is_file())
        second = subprocess.run(command, cwd=ROOT, text=True, capture_output=True, check=True)
        self.assertEqual(sample, deploy.load_state(out / "deployment.json"))
        self.assertNotIn("apt-get", second.stdout)

    def test_cli_bad_domain_stops_before_creating_output(self):
        out = self.path / "rejected"
        result = subprocess.run(["bash", str(ROOT / "deploy.sh"), "--dry-run", "--domain", "bad.com;id", "--email", "ops@example.com", "--output-dir", str(out)], cwd=ROOT, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(out.exists())

    def test_refuses_public_database_binding(self):
        for listen in ("localhost", "127.0.0.1, ::1"):
            deploy.check_postgres(listen)
        for listen in ("*", "0.0.0.0", "localhost,192.0.2.1", "", "::"):
            with self.subTest(listen=listen), self.assertRaises(ValueError):
                deploy.check_postgres(listen)

    def test_download_checksum_mismatch_is_rejected(self):
        archive = self.path / "download.tar.gz"
        archive.write_bytes(b"original archive")
        expected = hashlib.sha256(archive.read_bytes()).hexdigest()
        command = ["python3", str(ROOT / "deploy/render.py"), "verify", "--file", str(archive), "--sha256", expected]
        subprocess.run(command, check=True, capture_output=True)
        archive.write_bytes(b"modified archive")
        result = subprocess.run(command, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("校验失败", result.stderr)


if __name__ == "__main__":
    unittest.main()
