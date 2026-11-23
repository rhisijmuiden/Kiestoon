# Logs

**System › Logs** shows what the phone system and the portal write down. Use it to find out why something does not
work: a trunk that does not register, a phone that cannot sign in, a call that fails, a backup or mail that goes wrong.

![Logs](shot:logs)

## The logs

| Log | What is in it |
| --- | --- |
| **Phone system** | Notices, warnings and errors of Asterisk (`/var/log/asterisk/messages.log`): trunk registrations, calls that fail, phones that cannot sign in |
| **Security** | Sign-in attempts on phone accounts, also failed and blocked ones (`security.log`). The [firewall](firewall.md) (fail2ban) reads this log |
| **Portal** | Messages of the Kiestoon portal: mail, backups, updates, integrations |
| **System helper** | The service that applies the firewall, backups, internet access and updates as root |
| **Phone engine service** | Start, stop and crashes of the Asterisk service |

## Searching

- **Search** shows only the lines that contain all the words you type, for example a host name, a phone number,
  an extension or `trunk-3`. The words are highlighted.
- **Show** limits the lines to *notices and worse*, *warnings and errors* or *errors only*.
- **Follow** reloads the log every 5 seconds, handy while you try something on a phone.
- **Download** saves the whole log as a text file, for example to send to your supplier.

The newest line is at the top. The viewer reads the end of the log (the last few megabytes, including the previous
week when the log has just been rotated); for older lines use **Download**. The Asterisk logs are rotated weekly and
kept for 8 weeks.

## Why a trunk does not work

When a trunk is *Rejected*, *Not registered* or *Unreachable*, the [trunk list](trunks.md#status) shows the reason
under the status, read from the phone system log, for example *Sign-in refused (401): the user name or password is
wrong*. Click **Show log lines** for what to check and the log lines about that trunk, and **Open in logs** to see
everything around it.
