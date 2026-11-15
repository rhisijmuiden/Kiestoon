# Voicemail

Every extension can have a **voicemail box**. Callers leave a message when nobody answers, when the line is busy,
or when a route or IVR sends them straight to voicemail.

## Switch it on

In the extension form (see [Extensions](extensions.md)):

- **Voicemail** on, with a **PIN** of 4-8 digits (generated when empty).
- **Send voicemail to e-mail**: the message arrives as an audio attachment, with the caller, time and length.
  Needs an e-mail address on the extension and a [mail server](mail.md).
- **Delete after sending**: the message is then only kept in the e-mail.

With [speech-to-text](prompts.md#speech) on, the e-mail and the portal also show a **transcription** of the
message.

## Listen

| Where | How |
| --- | --- |
| **User portal › Voicemail** | Play messages in the browser, mark them as heard, delete them |
| **Phone** | Dial `*97` from your own phone, or the voicemail key |
| **Any phone** | Dial `*98`, enter the mailbox number and PIN |
| **E-mail** | The audio attachment |

![Voicemail in the user portal](shot:voicemail)

## Greetings

In **User portal › Voicemail › Greetings** employees record their own greetings in the browser:

| Greeting | Played |
| --- | --- |
| **Not available** | When you do not answer |
| **Busy** | When you are on another call |
| **Name** | Your name, used in standard messages |
| **Temporary** | Overrides the others while it exists, for example during a holiday |

Without an own greeting callers hear a standard message with the extension number, in the prompt language from
[Settings](settings.md). The PIN can be changed on the same page.
