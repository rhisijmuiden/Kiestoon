# Voice prompts and speech

**Voice prompts** are the announcements in IVR menus and queues: "Welcome to …", "All our colleagues are busy …".
The built-in system prompts (voicemail, queue positions, numbers, dates) come in English and Dutch; your own prompts
are made on this page or right inside the [IVR builder](ivr.md).

![Voice prompts with their waveform](shot:prompts)

## Add a prompt

Click **New prompt**, give it a name and choose one of:

- **Record** in the browser with your microphone (needs the portal over https),
- **Upload** an audio file (MP3, WAV, M4A, OGG),
- **Type the text**, choose the engine and voice and click **Generate audio** (when text-to-speech or ElevenLabs is on, see below).

Audio is converted in the browser to telephone quality (8 kHz mono WAV). To rename a prompt, leave the audio empty.

## Prompt language

The language of the built-in prompts is set under **Settings › Calls › Language of prompts**. Dutch prompts come
from the `kiestoon-sounds-nl` package, spoken by an open (CC0) Piper voice. Anything without a Dutch version falls
back to English.

## Speech

The **Speech** page (System menu) adds local speech technology: Piper and Whisper run on the server and **no audio leaves it**. Optional cloud voices are described under [ElevenLabs voices](#elevenlabs-voices).

![Speech: voices and Whisper models](shot:speech)

| Feature | What it does |
| --- | --- |
| **Text-to-speech (Piper)** | Type the text of a menu or announcement and Kiestoon speaks it with a natural voice. Download at least one voice per language and pick the **English voice** and **Dutch voice** |
| **Speech-to-text (Whisper)** | Voicemail messages are transcribed and shown in the user portal and the voicemail e-mail (the mail can arrive up to a minute later) |

Voices and models are downloaded from the portal with pinned SHA-256 checksums. Larger Whisper models are more
accurate but slower; *small* is a good choice from 2 CPU cores. Speech needs the `kiestoon-speech` package
(`apt install kiestoon-speech`, about 66 MB).

Summaries of calls and voicemail with a local model or Claude are on the [AI summaries](ai.md) page.

## ElevenLabs voices

Next to the local Piper voices, Kiestoon can have prompts spoken by [ElevenLabs](https://elevenlabs.io), a cloud
service with very natural voices. It is off by default and needs your own ElevenLabs account.

1. Create an API key at elevenlabs.io under **Developers › API keys**, with access to text-to-speech and voices.
2. Open **Speech**, switch on **Use ElevenLabs**, paste the key and click **Check and save key**. The key is checked
   with ElevenLabs, stored encrypted on the server and never shown again.
3. Under **Voice profile**, choose the **Dutch voice** and **English voice**, and set **Engine** to **ElevenLabs** to use
   it by default. Switch on **Lock the voice profile** to fix the voice for every prompt (see below).

![ElevenLabs settings on the Speech page](shot:speech)

**Choose a voice** shows the ElevenLabs voice library and the voices in your account. It is filtered on the language of
the PBX (**Settings › Calls › Language of prompts**), so a Dutch PBX shows Dutch voices first. Filter on gender, use and
accent, and click the play button to hear a free sample. A library voice is added to your ElevenLabs account when you
use it.

![Choosing a voice with samples](shot:voice-picker)

In the [IVR builder](ivr.md) and on this page, **Generate** starts with the voice of the profile and lets you choose
another engine or voice for one prompt, unless the profile is locked. The
counter under the text shows how many characters it costs and how many are left this period.

![Generating a menu prompt with ElevenLabs](shot:ivr-tts)

Good to know:

- Only the typed text goes to ElevenLabs. Callers, recordings and voicemail never leave the server.
- The audio is converted to 8 kHz WAV and stored on the server, so calls never wait for ElevenLabs and keep working
  without internet.
- The same text with the same voice and settings is reused from the server and costs no new characters.
- **Use Piper when ElevenLabs cannot be reached** keeps generating working when the key, the credits or the internet
  are gone (needs Piper switched on with a voice).
- The admins get a mail (to the notification address of the [mail server](mail.md)) when 80% of the characters of the
  period are used.
- **Model**: *Multilingual v2* sounds the most natural in Dutch; *Flash v2.5* is faster and costs half the characters.

### Voice profile

The **Voice profile** card on the **Speech** page, under the API key, holds everything ElevenLabs uses to speak a
prompt: the **Dutch voice** and **English voice**, the **Model** and the voice settings.

| Setting | What it does |
| --- | --- |
| **Stability** | Higher sounds steadier; lower sounds livelier. |
| **Similarity** | How closely the result follows the original voice. 75% is a good start. |
| **Style exaggeration** | Adds expression. Keep it at 0% for calm menu announcements. |
| **Speed** | 95% is a little slower than normal, pleasant for menu choices. |
| **Speaker boost** | Makes the voice a little clearer. Recommended on the phone. |

Switch on **Lock the voice profile** so all prompts sound like the same speaker. Every **Generate** button (the
IVR builder, this page and **Generate again with this voice**) then uses the profile's voice for the language of the
text, with these settings; the engine and voice choice is replaced by the locked profile with a link back here. The
lock needs ElevenLabs switched on, a saved key and at least one voice. When a language has no voice in the profile,
the other voice speaks it. Switching ElevenLabs off releases the lock.

![The voice profile, locked](shot:voice-profile)

### Another voice for all prompts

Prompts made from text remember their text and voice (the **Source** column). Tick them on this page, choose an engine
and voice, and click **Generate again with this voice** to speak them all again, for example after choosing a new
house voice. Recorded and uploaded prompts cannot be ticked.
