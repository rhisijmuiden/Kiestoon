# Troubleshooting

Start with the **dashboard**: it shows whether the phone engine runs, which trunks are online, how many phones are
registered and any warning that needs attention.

## Phones

??? question "A phone does not register"
    - Check the extension number and SIP password (**Extensions › Edit**, or the SIP details in the user portal).
    - Is the phone outside the office? Then its address must be allowed in the [firewall](firewall.md): use the SIP
      whitelist, **Allow SIP from local networks** or an [Edge](edge.md) for branch offices.
    - Look under **Firewall › Blocked addresses**: after too many wrong passwords fail2ban blocks the address.
      **Unblock** lifts it.
    - Geo-blocking can keep out a phone abroad.

??? question "A phone is not provisioned automatically"
    - Point the phone at `http://<server>:8080/prov/` (DHCP option 66, or the phone's own provisioning setting).
    - New phones appear under **Waiting for an extension**: choose an extension and click **Link**.
    - Check the provisioning username and password under **Phones › Provisioning settings**.
    - **Preview config** shows the exact file the phone gets.

??? question "One-way audio or no audio"
    Almost always NAT. Fill in the **Public address** and **Local networks** under [Settings](settings.md), and
    forward the audio ports (default UDP 10000–20000) to the server. Switch off "SIP ALG" on the router.

## Calls

??? question "Outbound calls fail"
    - Is the trunk online on the dashboard? Check the trunk credentials and the provider's IP whitelist.
    - Does an [outbound route](routes.md) match the dialled number, and may the extension dial it
      (**Outbound calls** on the extension)?
    - Did fraud protection block outbound calls? Then the dashboard and **Call features & fraud** show it, with an
      **Unblock** button.

??? question "Incoming calls do not arrive"
    - Does the [inbound route](routes.md) have the number exactly as the provider sends it? The trunk setting **Called
      number comes from** decides where Kiestoon reads the number.
    - Check the [time condition](time-conditions.md): maybe the call goes to the closed destination, or the mode is
      forced to night.

??? question "Callers hear English prompts"
    Set **Language of prompts** to Dutch under [Settings](settings.md), and check the language of IVR and queue
    prompts under [Voice prompts](prompts.md).

## Portal and softphone

??? question "The browser softphone says 'Needs https'"
    Browsers only allow the microphone over https. Open the portal through its [internet address](internet.md) or
    a [reverse proxy](reverse-proxy.md), not via `http://<ip>:8080`.

??? question "Passkeys cannot be added"
    Passkeys need https and a host name, not an IP address.

??? question "I locked myself out with the firewall"
    Safe mode undoes a change within 90 seconds when you cannot reach the portal. Locked out anyway? On the
    server console, `nft delete table inet kiestoon` removes the Kiestoon rules until the next change in the
    portal.

??? question "I lost my MFA phone"
    Sign in with one of your recovery codes. Another administrator can also use **Reset MFA** under
    [Users & roles](users.md).

## Mail and backups

??? question "No voicemail e-mails"
    Send a test under [Mail server](mail.md) and check **Recent e-mails** for the error. Check that the extension
    has an e-mail address and **Send voicemail to e-mail** on.

??? question "A backup failed"
    **Recent backups** shows the error. Click **Test** on the location: often it is a changed password, a full
    disk or a firewall in between.

## Logs on the server

| What | Command |
| --- | --- |
| Portal | `journalctl -u kiestoon -f` |
| Phone engine | `journalctl -u asterisk -f`, or `asterisk -rvvv` for the live console |
| SIP registrations | `asterisk -rx "pjsip show contacts"` |
| Updates | `journalctl -u kiestoon-updater` |
| Firewall | `nft list ruleset` |
| fail2ban | `fail2ban-client status` |
| Version | `kiestoon version` |
| Setup token | `sudo kiestoon setup-token` |

Still stuck? Open an [issue on GitHub](https://github.com/rhisijmuiden/Kiestoon/issues) with the version and what
you see in the logs. Remove passwords and phone numbers first.
