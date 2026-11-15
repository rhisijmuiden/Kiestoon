# Fax

Kiestoon receives faxes and mails them as PDF (**fax-to-mail**), and sends PDFs as fax from the portal or by e-mail
(**mail-to-fax**). No fax machine needed.

![Received faxes](shot:fax)

## Receive faxes

1. Open **Fax › Fax boxes** and click **Add fax box**.
2. Fill in the **e-mail addresses** that get the faxes (up to 10, separated by commas), how long to **keep** them
   (0: until you delete them) and leave **Error correction (ECM)** on.
3. Add an [inbound route](routes.md) for your fax number with the fax box as destination.
4. Switch on **T.38 fax** on the trunk when your provider supports it: it is more reliable than fax over G.711.

Each received fax is converted to PDF and mailed. The **Received** tab lists all faxes with sender, number, pages
and status; download the PDF from there.

## Send faxes from the portal

1. On the fax box, switch on **Allow sending from this box** and fill in the **fax number** (shown to the receiver
   and in the fax header).
2. Users with access open **Fax › Send**, choose the box, the fax number and a PDF (max 11 MB) and click **Send
   fax**.
3. Kiestoon converts the PDF, dials, and tries again up to three times when the line is busy. At most two faxes are
   sent at once. The sender gets an e-mail with the result.

## Mail-to-fax

1. Use a separate mailbox, for example `fax@yourcompany.com`.
2. On **Fax › E-mail to fax**, fill in its IMAP server and sign in. Kiestoon checks it every minute.
3. Add the **allowed e-mail senders** to a fax box that may send, and optionally a **PIN in the subject**.
4. Send an e-mail with the fax number in the subject (or to `fax+0201234567@yourcompany.com` when your mail server
   supports plus addresses) and one or more PDFs attached.

!!! warning "Safety"
    Mail that fails the SPF or DMARC check of your mail server is ignored, so a forged sender cannot send faxes.
    Sending depends on T.38 support of your SIP provider.
