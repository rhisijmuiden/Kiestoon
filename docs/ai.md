# AI summaries

Kiestoon can write out recorded calls and voicemail messages and summarise them: what the call was about,
what was agreed, and which follow-ups somebody has to do. Open **AI** in the System menu.

![AI: what to summarise and which engine](shot:ai)

## How it works

1. **The transcript is always made on the server** by Whisper (see [Speech](prompts.md#speech)). Audio never
   leaves the phone system.
2. **The summary** comes from the engine you choose:

| Engine | What it means |
| --- | --- |
| **Local model** (default) | A language model runs on the server itself with llama.cpp. Free and private: nothing leaves the server. It is slower and needs free memory (about 4 GB for Qwen3 4B, 2 GB for Qwen3 1.7B). The model starts when there is work and stops after 5 idle minutes |
| **Claude (cloud)** | Faster and better summaries from Anthropic's Claude, with your **own API key** (create one at console.anthropic.com). Only the text of the transcript is sent; usage is billed by Anthropic to your account |

The API key is stored apart from the other settings and is never shown again after you save it (you see only
its last four characters). Remove it with **Remove key**.

## What to summarise

| Setting | What it does |
| --- | --- |
| **Recorded calls** | New recordings (of calls that are recorded anyway) are transcribed and summarised in the background, newest first, one at a time |
| **Voicemail** | Voicemail messages get a summary and follow-ups in the user portal, the Windows softphone and the voicemail e-mail |
| **Language of the summaries** | *Same as the call*, or always Dutch or English |
| **Keep summaries of calls** | Removes summaries and transcripts of calls after this many days. 0 keeps them as long as the recording exists |

Press **Try it** to summarise a sample voicemail with the current settings before you save them.

## Summaries of recordings

Under **Recordings** (and **My calls** in the portal) a call with a summary shows its topic and the caller's mood.
Click the sparkle button to open the summary, the follow-ups and the full transcript. Reception and administrators
can also **Transcribe and summarise** a single recording right away, even when automatic summaries are off.

![A recording with its AI summary](shot:recording-ai)

!!! note "Speech recognition can make mistakes"
    A summary is only as good as the transcript. Names and numbers are worth checking against the recording.
    The model is told not to invent anything that is not in the transcript.

## Requirements

- The `kiestoon-speech` package (version 1.1.0 or newer has llama.cpp for the local model).
- Speech-to-text turned on under **Speech**, with a Whisper model downloaded.
- For the local model: the model downloaded on the AI page (pinned SHA-256 checksums).
