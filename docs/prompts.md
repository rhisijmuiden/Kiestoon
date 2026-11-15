# Voice prompts and speech

**Voice prompts** are the announcements in IVR menus and queues: "Welcome to …", "All our colleagues are busy …".
The built-in system prompts (voicemail, queue positions, numbers, dates) come in English and Dutch; your own prompts
are made on this page or right inside the [IVR builder](ivr.md).

![Voice prompts with their waveform](shot:prompts)

## Add a prompt

Click **New prompt**, give it a name and choose one of:

- **Record** in the browser with your microphone (needs the portal over https),
- **Upload** an audio file (MP3, WAV, M4A, OGG),
- **Type the text** and click **Generate audio** (when text-to-speech is on, see below).

Audio is converted in the browser to telephone quality (8 kHz mono WAV). To rename a prompt, leave the audio empty.

## Prompt language

The language of the built-in prompts is set under **Settings › Calls › Language of prompts**. Dutch prompts come
from the `kiestoon-sounds-nl` package, spoken by an open (CC0) Piper voice. Anything without a Dutch version falls
back to English.

## Speech

The **Speech** page (System menu) adds local speech technology. **No audio leaves the server.**

![Speech: voices and Whisper models](shot:speech)

| Feature | What it does |
| --- | --- |
| **Text-to-speech (Piper)** | Type the text of a menu or announcement and Kiestoon speaks it with a natural voice. Download at least one voice per language and pick the **English voice** and **Dutch voice** |
| **Speech-to-text (Whisper)** | Voicemail messages are transcribed and shown in the user portal and the voicemail e-mail (the mail can arrive up to a minute later) |

Voices and models are downloaded from the portal with pinned SHA-256 checksums. Larger Whisper models are more
accurate but slower; *small* is a good choice from 2 CPU cores. Speech needs the `kiestoon-speech` package
(`apt install kiestoon-speech`, about 66 MB).
