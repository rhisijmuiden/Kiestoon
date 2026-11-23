# Extension groups

An **extension group** is a set of extensions that call out and work the same way, for example *Sales* or
*Support*. One group can do three things for all its members at once:

- send **its own number** on outside calls, and get **its own outbound route** (for example its own trunk),
- **enforce settings** such as voicemail, recording and outbound class,
- keep the **keys of their phones** the same with a key template.

Open **Extensions › Extension groups**.

![Extension groups with their number, enforced settings and own routes](shot:extension-groups)

## Make a group

1. Click **New group** and give it a **name**.
2. Pick an **outbound caller ID** (the numbers of your trunks are suggested), or leave it empty.
3. Click the **members**. An extension is in one group at a time; choosing one that is in another group moves it.
4. Tick the **settings for all members** you want to enforce (see below) and pick **key templates** if you like.
5. Click **Save**. Kiestoon rebuilds the phone configuration and sends the keys to the members' phones.

![Editing a group: number, members, enforced settings and key templates](shot:extension-group-edit)

You can also put a single extension in a group from its own form, with **Extension group**.

## Which number goes out

On an outside call Kiestoon sends the first of these that is filled in:

1. the **extension's own** outbound caller ID,
2. the **group's** caller ID,
3. the **outbound route's** caller ID,
4. the **trunk's** caller ID.

So a group number is the default for everyone in the group, and one person (a manager with a direct number) can
still have their own. The extension form says which number it sends when its own field is empty.

## Own outbound route per group

Under **Trunks › Outbound routes**, **Who can use this route** limits a route to one or more groups. Members of
those groups use it; everyone else skips it and takes the next route that matches. Put the group route **above**
the general route.

Example: *Support line* only for the group Support, over its own trunk, above *National and mobile* for everyone.

![An outbound route that only the group Support uses](shot:outbound-route-groups)

A group that a route is limited to cannot be deleted; take it out of the route first. When no route is left for a
caller, the call ends with a "no service" message.

## Settings for all members

Tick a setting to enforce it on every member. Settings you leave unticked stay **per extension**.

| Setting | |
| --- | --- |
| **Voicemail**, **Send voicemail to e-mail**, **Delete after sending** | Voicemail to e-mail only works for members with an e-mail address |
| **Ring time (seconds)**, **Devices at the same time** | |
| **Outbound calls**, **Allow premium-rate numbers** | |
| **Recording** | Never, on request or always |
| **Pickup group** | |
| **Browser softphone** | |

An enforced setting is locked in the extension form, with the group and the value next to it:

![A locked setting in the extension form](shot:extension-locked)

When you take an extension out of the group (or delete the group), it keeps the values it has; you can change them
again per extension.

## Phone keys for the whole group

Pick a [key template](phone-keys.md) per phone model. When you save the group, every phone of a member of that
model gets the template's keys and is asked to fetch them. A phone that is linked to a member later, or an
extension that joins the group later, gets them too. **Send the keys to the phones again** on the row repeats it,
for example after you changed the template.
