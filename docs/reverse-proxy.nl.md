# Eigen reverse proxy

Kiestoon kan zichzelf op internet zetten met Caddy en een Let's Encrypt-certificaat ([Internettoegang](internet.md)).
Heb je al een reverse proxy of load balancer (nginx, Traefik, HAProxy, een firewall met webproxy), laat dat dan uit
en laat je proxy doorsturen naar Kiestoon.

## 1. Vertel Kiestoon welke proxy het mag vertrouwen

Ga naar **Internettoegang › Eigen reverse proxy**:

1. Zet **Mijn reverse proxy vertrouwen** aan.
2. Vul de **Adressen van de proxy** in (of de reeks van een proxycluster), één per regel. Kiestoon leest
   `X-Forwarded-For` en `X-Forwarded-Proto` alleen van deze adressen. Van elk ander adres worden de headers
   genegeerd, dus niemand kan zijn adres vervalsen om de inlogblokkade of de firewall te omzeilen.
3. Vul het **Publiek adres** in, zoals `https://pbx.example.com`. Kiestoon gebruikt het in e-mails, voor
   zero-touch-provisioning en voor de firmwarelinks die het aan telefoons geeft.
4. Sla op en open het portaal via de proxy. Het blok **Wat Kiestoon nu ziet** moet je eigen adres tonen als
   "Jouw adres" en HTTPS als verbinding.

Regels die Kiestoon volgt:

- `X-Forwarded-For` wordt van rechts naar links gelezen; vertrouwde proxy's worden overgeslagen en het eerste andere
  adres is de client. Een keten van proxy's werkt als ze allemaal in de lijst staan.
- Verzoeken van `127.0.0.1` en `::1` worden altijd vertrouwd (een proxy op dezelfde server).
- Een reeks `/0` (alles) wordt geweigerd.

## 2. Doorsturen naar Kiestoon

Stuur door naar `http://<kiestoon>:8080` op je eigen netwerk, of naar `https://<kiestoon>:8443` (zelfondertekend
certificaat) als het netwerk tussen proxy en Kiestoon niet vertrouwd is.

Je proxy moet:

- `X-Forwarded-For` en `X-Forwarded-Proto` zetten;
- de `Host`-header ongewijzigd doorgeven (passkeys en single sign-on controleren hem);
- **WebSockets** doorsturen op `/api/webrtc/ws`, voor de softphone in de browser;
- uploads tot **1 GB** toestaan op `/api/firmware/upload` en 16 MB elders, met een lange timeout (firmware uploaden
  kan minuten duren).

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

# in het http-blok:
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

Traefik zet `X-Forwarded-*` en stuurt WebSockets zelf door. Verhoog de timeouts van het entry point
(`respondingTimeouts.readTimeout`) als je grote firmwarebestanden uploadt.

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

### Caddy op een andere server

```caddy
pbx.example.com {
    reverse_proxy 10.0.0.20:8080
}
```

Caddy zet de headers en regelt WebSockets zelf.

## 3. Wat niet via de proxy gaat

Een webproxy draagt alleen het portaal, de signalering van de softphone en provisioning van telefoons via HTTPS.
Telefoons en trunks bereiken Asterisk nog steeds rechtstreeks:

| Poort | Wat |
| --- | --- |
| 5060 UDP/TCP, 5061 TCP | SIP (5061 is SIP over TLS) |
| 10000–20000 UDP | Audio (RTP) |

Stuur die op je router of firewall door naar Kiestoon, of houd telefoons op het lokale netwerk of een VPN.

## Prometheus achter de proxy

`/metrics` gaat via de proxy zoals elk ander pad. Gebruik het bearer-token, of zet het adres van de
Prometheus-server bij "Toegestaan zonder token". Die lijst wordt vergeleken met het clientadres dat Kiestoon afleidt
via de vertrouwde proxy, niet met de proxy zelf.
