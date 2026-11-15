# Users, roles and sign-in

Everyone who opens the portal signs in with their own account. The **role** of the account decides what they see.

## Roles

| Role | What they can do |
| --- | --- |
| **Super admin** | Full control, including other super admins. |
| **Administrator** | Manages the phone system. Must use two-factor authentication. |
| **Reception** | Sees live calls and queues, signs agents in and out, switches day and night mode, uses the switchboard and wallboard. |
| **User** | Manages their own phone in the [user portal](portal.md). |

An account linked to an **extension** also gets the user portal of that extension, whatever its role. An
administrator with an extension sees both menus.

![Users & roles](shot:users)

## Add employees

There are two ways:

- **With the extension.** In the [extension](extensions.md) form, switch on **Create a portal account and send a
  welcome e-mail**. The employee gets an e-mail with a link to set a password (valid for 72 hours), the portal
  address and the details for a softphone app. No password is ever sent by e-mail. **Send welcome e-mail** sends a
  new link later.
- **Under Users & roles › New user**, with a username, full name, e-mail, role and optionally an extension. Leave
  the password empty for accounts that only use single sign-on.

A disabled account cannot sign in. **Reset MFA** switches off the second factor of an account, for example after
a lost phone; the person sets it up again at the next sign-in.

## Two-factor authentication (MFA)

Administrators and super admins must use a second factor. At their first sign-in Kiestoon shows a QR code for an
authenticator app (Microsoft Authenticator, Google Authenticator, 1Password, Bitwarden…). After confirming the
first code they get **recovery codes**: each works once, for when the phone is lost. Store them in a password
manager.

A passkey, or single sign-on with **Trust the provider's MFA**, counts as the second factor as well.

## Passkeys

Under **My account › Passkeys** everybody can add a passkey: sign in with a fingerprint, face or device PIN
instead of a password. Passkeys only work over **https** with a host name (for example
`https://pbx.example.com`), not with an IP address. See [Internet access](internet.md).

![My account](shot:account)

## Single sign-on

![Single sign-on](shot:sso)

Under **Single sign-on › New provider** you connect an identity provider. The sign-in page then shows a button
"Sign in with …".

| Provider | Issuer URL |
| --- | --- |
| **Microsoft Entra ID** | `https://login.microsoftonline.com/<tenant-id>/v2.0` |
| **Google Workspace** | Fixed for Google |
| **Keycloak** | Your Keycloak address with the realm name |
| **authentik** | Ends with the slug of the application |
| **Other (OpenID Connect)** | The address that serves `/.well-known/openid-configuration` |
| **SAML 2.0** | The metadata URL of the identity provider |

### Microsoft Entra ID step by step

1. In the Entra admin center, go to **App registrations › New registration**.
2. Add the **Redirect URI** that Kiestoon shows, with type **Web**.
3. Create a **client secret** under *Certificates & secrets*.
4. Under *Token configuration*, add a **groups claim** (security groups).
5. In Kiestoon, fill in the tenant ID in the issuer URL, the **Client ID (application ID)** and the **Client
   secret**.
6. Under **Role mapping**, map groups to roles. For Entra ID use the group's **object ID**. Members get the role
   of their group; the highest role wins.
7. Choose the **Role without a matching group**: a role, or **No access** to refuse everyone outside the groups.

Other options:

- **Create accounts automatically**: at the first sign-in. Off: an administrator creates the account first, with
  the same e-mail address.
- **Trust the provider's MFA**: administrators who signed in with SSO need no second Kiestoon code. Enforce MFA
  at the provider then, for example with Conditional Access.

For **SAML 2.0**, give the identity provider the **SAML entity ID / metadata** and the **SAML reply URL (ACS)**
that Kiestoon shows, and fill in the metadata URL (or paste the XML). Groups come from the **Groups attribute**.

## Audit log

The **Audit log** shows the last 200 events, with time, user and IP address: every sign-in and failed attempt,
every change to the configuration, and sensitive actions such as playing a recording, exporting call history,
revealing the backup key, a restore, an update or a fraud alert.

![Audit log](shot:audit)
