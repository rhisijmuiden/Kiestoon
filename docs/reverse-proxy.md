# Own reverse proxy

Kiestoon can put itself on the internet with Caddy and a Let's Encrypt certificate ([Internet access](internet.md)). When you already have a reverse proxy or load balancer (nginx, Traefik, HAProxy, a firewall with a
web proxy), keep that switched off and let your proxy forward to Kiestoon instead.

## 1. Tell Kiestoon which proxy to trust

Go to **Internet access › Own reverse proxy**:

1. Switch on **Trust my reverse proxy**.
2. Enter the IP address of the proxy (or the range of a proxy cluster), one per line. Kiestoon only reads
   `X-Forwarded-For` and `X-Forwarded-Proto` from these addresses. From any other address the headers are
   ignored, so nobody can fake their address to get around the sign-in lockout or the firewall.
3. Enter the **public address**, such as `https://pbx.example.com`. Kiestoon uses it in e-mails, for
   zero-touch provisioning and for the firmware links it gives phones.
4. Save, then open the portal through the proxy. The box **What Kiestoon sees now** must show your own
   address as "Your address" and HTTPS as the connection.

Rules Kiestoon follows:

- `X-Forwarded-For` is read from right to left; trusted proxies are skipped and the first other address is
  the client. A chain of proxies works when all of them are in the list.
- Requests from `127.0.0.1` and `::1` are always trusted (a proxy on the same server).
- A range of `/0` (everything) is refused.

## 2. Forward to Kiestoon

Forward to `http://<kiestoon>:8080` on your own network, or to `https://<kiestoon>:8443` (self-signed
certificate) when the network between proxy and Kiestoon is not trusted.

Your proxy must:

- set `X-Forwarded-For` and `X-Forwarded-Proto`;
- pass the `Host` header unchanged (passkeys and single sign-on check it);
- forward **WebSockets** on `/api/webrtc/ws`, for the browser softphone;
- allow uploads of up to **1 GB** on `/api/firmware/upload` and 16 MB elsewhere, with a long timeout
  (firmware uploads can take minutes).

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name pbx.example.com;
    ssl_certificate     /etc/ssl/pbx.example.com/fullchain.pem;
    ssl_certificate_key /etc/ssl/pbx.example.com/privkey.pem;

    client_max_body_size 1g;
    proxy_read_timeout   3600s;
    proxy_send_timeout   3600s;

    location / {
        proxy_pass http://10.0.0.20:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;
        proxy_buffering off;
        proxy_request_buffering off;
    }
}

# in the http block:
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

### Traefik (file provider)

```yaml
http:
  routers:
    kiestoon:
      rule: Host(`pbx.example.com`)
      entryPoints: [websecure]
      tls:
        certResolver: letsencrypt
      service: kiestoon
  services:
    kiestoon:
      loadBalancer:
        servers:
          - url: http://10.0.0.20:8080
        passHostHeader: true
```

Traefik sets `X-Forwarded-*` and forwards WebSockets by itself. Raise the timeouts of the entry point
(`respondingTimeouts.readTimeout`) when you upload large firmware files.

### HAProxy

```haproxy
frontend https
    bind :443 ssl crt /etc/haproxy/certs/pbx.example.com.pem alpn h2,http/1.1
    http-request set-header X-Forwarded-Proto https
    option forwardfor
    timeout client 1h
    default_backend kiestoon

backend kiestoon
    timeout server 1h
    timeout tunnel 1h
    server pbx 10.0.0.20:8080 check
```

### Caddy on another server

```caddy
pbx.example.com {
    reverse_proxy 10.0.0.20:8080
}
```

Caddy sets the headers and handles WebSockets by itself.

## 3. What does not go through the proxy

A web proxy only carries the portal, the softphone signalling and phone provisioning over HTTPS. Phones and
trunks still reach Asterisk directly:

| Port | What |
| --- | --- |
| 5060 UDP/TCP, 5061 TCP | SIP (5061 is SIP over TLS) |
| 10000–20000 UDP | Audio (RTP) |

Forward those on your router or firewall to Kiestoon, or keep phones on the local network or a VPN.

## Prometheus behind the proxy

`/metrics` goes through the proxy like every other path. Use the bearer token, or add the address of the
Prometheus server to "allowed without a token". That list is checked against the client address Kiestoon
derives from the trusted proxy, not against the proxy itself.

!!! tip "Two host names"
    Want a separate address for the employee portal, such as `voipcentrale.example.com` next to
    `sip.example.com`? Fill in **Employee portal address** too, see [Separate host names](hostnames.md).
