# Before You Begin

This document goes over rudimentary settings options in the config.yaml file. It is meant for users that will be setting up
their own mesh network as the seed/admin node.  For users that just want to join an existing mesh ask the owner for an invite link
and run 

```
mesh proxy join <LINK>
```

# Setting Up a Seed/Admin node

Seed/Admin nodes are the initial nodes in the mesh network used for bootstrapping all client/peer nodes going forward.
The seed/admin node should have a public IP address, or at least be able to have the required ports (4001,4002) 
forwarded to host running the admin service.

## Considerations

It is ***STRONGLY*** recommended that the seed/admin API port (admin_port: 4002) be protected with an SSL certificate 
using a reverse proxy like nginx.  Otherwise your admin requests are going to be plain text over the internet.

**Note: we will support native SSL certificates at a later time.**  

The mesh relay port (relay_port: 4001) is by default encrypted as it uses the QUIC protocol.

## Create an Initial Config

```aiignore
./mesh admin init
```

This will create a barebones config with default settings.  You can then edit the config.yaml file to your liking.

This is the bare minimum config required for hosting a public seed/admin node fronted by a reverse proxy:
```config.yaml
admin:
  address: "https://myhost.mydomain.com"
  secret: "mysekrit"
  admin_port: 4002
  relay_port: 4001
  public_address: "auto"
```

In the above example https://myhost.mydomain.com is being reverse proxied to our server using nginx: 

```aiignore
server {
    server_name myhost.mydomain.com;

    root /var/www/myhost.mydomain.com;
    index index.html;

    location / {
        proxy_pass http://127.0.0.1:4002;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }


    listen [::]:443 ssl ipv6only=on; # managed by Certbot
    listen 443 ssl; # managed by Certbot
    ssl_certificate /etc/letsencrypt/live/myhost.mydomain.com/fullchain.pem;   # managed by Certbot
    ssl_certificate_key /etc/letsencrypt/live/myhost.mydomain.com/privkey.pem; # managed by Certbot
    include /etc/letsencrypt/options-ssl-nginx.conf; # managed by Certbot
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem; # managed by Certbot
}
```

you can then start the peer/admin server with

```aiignore
./mesh admin start
```

With the server runing create your first mesh network invite

```aiignore
./build/admincli --addr https://myhost.mydomain.com --token "mysekrit" admin invite --name "testinvite" 
invite id:   7894980ad0729892a0655cef16b53d5df9b9cdccf2e1e1b19d4497c8eefae1bc
invite link: https://myhost.mydomain.com/api/v1/redeem/7894980ad0729892a0655cef16b53d5df9b9cdccf2e1e1b19d4497c8eefae1bc
uses:        unlimited
expires:     2026-09-11T04:50:45Z
```

Anyone with this invite link can now join your mesh network by executing

```aiignore
./mesh proxy join https://myhost.mydomain.com/api/v1/redeem/7894980ad0729892a0655cef16b53d5df9b9cdccf2e1e1b19d4497c8eefae1bc
```

