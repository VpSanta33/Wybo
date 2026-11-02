import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location("wybo_deploy_stations",ROOT/"deploy/render.py")
deploy=importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)
cred_spec=importlib.util.spec_from_file_location("wybo_cf_credentials",ROOT/"deploy/station_credentials.py")
credentials=importlib.util.module_from_spec(cred_spec)
cred_spec.loader.exec_module(credentials)

class StationDeployTest(unittest.TestCase):
 def test_wildcard_upgrade_preserves_secrets_and_hosts(self):
  with tempfile.TemporaryDirectory() as folder:
   root=Path(folder);state_path=root/"deployment.json"
   original=deploy.init_state(state_path,"shop.example.com","ops@example.com","admin")
   subprocess.run(["python3",str(ROOT/"deploy/render.py"),"stations","--state",str(state_path),"--root","example.com"],check=True,capture_output=True)
   state=deploy.load_state(state_path)
   for key in ("jwt_secret","db_password","admin_password"):self.assertEqual(original[key],state[key])
   deploy.render(state,root/"out")
   nginx=(root/"out/nginx-https.conf").read_text()
   self.assertIn("server_name shop.example.com *.example.com;",nginx)
   self.assertIn("return 308 https://$host$request_uri",nginx)
   self.assertIn("proxy_set_header Host $host",nginx)
   config=json.loads((root/"out/config.yaml").read_text())
   self.assertEqual(config["server"]["station_root"],"example.com")
   self.assertEqual(config["server"]["public_host"],"shop.example.com")
   before=state_path.read_bytes()
   r=subprocess.run(["python3",str(ROOT/"deploy/render.py"),"stations","--state",str(state_path),"--root","attacker.test"],capture_output=True)
   self.assertNotEqual(r.returncode,0);self.assertEqual(before,state_path.read_bytes())
 def test_wildcard_dry_run_needs_no_dns_secret(self):
  with tempfile.TemporaryDirectory() as folder:
   p=subprocess.run(["bash",str(ROOT/"deploy.sh"),"--dry-run","--domain","shop.example.com","--email","ops@example.com","--station-root","example.com","--output-dir",folder],capture_output=True,text=True)
   self.assertEqual(p.returncode,0,p.stderr)
   self.assertEqual(json.loads((Path(folder)/"deployment.json").read_text())["station_root"],"example.com")
 def test_dns_credentials_permissions_and_no_symlinks(self):
  with tempfile.TemporaryDirectory() as folder:
   source=Path(folder)/"source.ini";target=Path(folder)/"target.ini"
   source.write_text("dns_cloudflare_api_token = "+"a"*40+"\n");source.chmod(0o644)
   with self.assertRaises(ValueError):credentials.prepare(source,target)
   source.chmod(0o600);credentials.prepare(source,target)
   self.assertEqual(target.stat().st_mode&0o777,0o600)
   link=Path(folder)/"link.ini";link.symlink_to(source)
   with self.assertRaises(ValueError):credentials.prepare(link,target)
