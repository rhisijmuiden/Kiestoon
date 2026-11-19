# Phone book

The **phone book** holds shared contacts for everyone: in the portal, on the desk phones (XML), over LDAP, and for
the names of incoming callers.

![The phone book](shot:phonebook)

## Contacts

- **Add contact**: name, company, work, mobile and other number, e-mail and notes.
- **Search** by name, company or number. Click a number to call it: your own phone rings first, then the number is
  called.
- **Import** a CSV file with a header row: `name, company, work, mobile, other, email, notes` (Dutch column names
  and `;` work too). Choose **Replace the whole phone book** or add and skip duplicates.
- **Export** the phone book as CSV.

## Settings

| Setting | Effect |
| --- | --- |
| **Include extensions** | Colleagues appear in the phone book with their extension |
| **Show names of incoming callers** | When a caller's number is in the phone book, the phone shows the name |
| **Phone book on the phones (XML)** | Yealink, Snom, Grandstream and Fanvil get the phone book through provisioning |
| **LDAP directory** | For phones and programs that look up names over LDAP (389, LDAPS 636). Only reachable from your own network; optionally with a password (the provisioning user) |

The settings card shows the server, base DN, bind DN and search filter to enter on a phone or program that uses
LDAP.

With [linked systems](multi-site.md) the extensions of the other sites are part of the phone book too.
