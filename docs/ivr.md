# IVR builder

The **IVR builder** is your digital receptionist: "Welcome to Noordkade. For sales press 1, for support press 2."
You build the menu visually: drag steps onto a canvas, connect them by drawing lines, and give every step its own
audio.

![The IVR canvas: keypresses, audio, transfers and a repeat step](shot:ivr-canvas)

## Create a menu

1. Open **IVR builder** and click **New IVR menu**. The canvas opens with one step: **Incoming call**.
2. Click **Incoming call** and give the menu a **name** in the side panel. Set:
    - **Wait for a key**: seconds the menu waits after the announcement,
    - **Repeat menu**: how many times the menu plays again before giving up,
    - **Allow dialing an extension**: callers who know an extension can dial it directly.
3. Give the incoming call its **audio message** (the welcome text), see [Audio per step](#audio-per-step).
4. Drag building blocks from the bar above the canvas (or click them to add), and connect them.
5. Click **Save**, then **Test call**.

Blocks show live details: whether a time condition is open right now, how many agents of a queue are online and
whether an extension is available. The buttons at the right of the bar switch between flowing and angular lines and
turn the moving dots on the lines (the direction of the call) on or off.

![The list of IVR menus](shot:ivr-list)

## Building blocks

| Block | What it does | Lines out |
| --- | --- | --- |
| **Incoming call** | Start of the menu; plays the welcome message | To the keypresses and *No input* |
| **Keypress** | "Press 1": the caller pressed this key | One |
| **No input** | The caller pressed nothing or an unused key, after the menu repeated | One |
| **Play audio** | Plays a message, then follows the line | One |
| **Speech question** | Asks what the caller wants and listens to the answer (see [below](#speech-question)) | One per answer, plus *Not understood* |
| **Extension, Ring group, Queue, Voicemail** | Transfers the call there | End point |
| **Submenu** | Another IVR menu | End point |
| **External number** | Forwards to a phone number outside | End point |
| **Time condition** | Checks the opening hours and the day/night switch | Two: *open* and *closed* |
| **Repeat menu** | Back to the start of the menu | End point |
| **Hang up** | Ends the call | End point |

Linked systems add **Other site**, and [follow-the-sun](multi-site.md#follow-the-sun) routes can be a target too.

## Drawing lines

Drag from the dot on the right of a step to the next step. A keypress only belongs to the menu when a line runs
from **Incoming call** to it. A time condition has two dots: **open** and **closed**.

The canvas helps you along:

- Steps with a problem get a warning: *Not part of the menu*, *No line leads to this step*, *No line goes out: the
  call ends here*, *Choose where the call goes*.
- **Undo** / **Redo** (also <kbd>Ctrl</kbd>+<kbd>Z</kbd> / <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Z</kbd>), **Tidy up** lays the
  flow out neatly, the minimap and zoom buttons help with large menus.
- **Duplicate** in the side panel copies a step.
- Keyboard: press <kbd>Enter</kbd> on a step to select it, then move it with the arrow keys.

## Audio per step

Every step that plays something (incoming call, keypress, no input, play audio) has its own audio message, with a
waveform player to listen to it. Choose the source in the side panel:

| Source | How |
| --- | --- |
| **Generate** | Type the text, pick a voice and click **Generate audio**. Spoken by the local speech engine (Piper); nothing leaves the server. Needs [Speech](prompts.md#speech) switched on |
| **Record** | Record with the microphone in the browser (needs https) |
| **Upload** | Drop an MP3, WAV, M4A or OGG file (max 10 MB). Converted to telephone quality |
| **Library** | Pick an existing [voice prompt](prompts.md) |
| **None** | No audio |

Generated, recorded and uploaded audio is saved under **Voice prompts**, so you can reuse it in queues and other
menus.

## Speech question

A **Speech question** lets callers say what they are calling about instead of pressing keys
("Please tell us in a few words what you are calling about").

![A menu with a speech question](shot:ivr-speech)

1. The step plays its question, then records the answer (up to 12 seconds; 3 seconds of silence ends it).
2. The answer is written out **on this server** by Whisper, while the caller hears hold music for a moment.
3. Every **answer** of the step has a name and keywords. The answer with the most keywords that occur in what
   the caller said wins. A keyword also matches longer words (*factu* matches *factuur* and *facturen*);
   capitals and accents do not matter, and a keyword of several words (*werkt niet*) must occur in that order.
4. With **Let AI choose when no keyword matches**, the [AI engine](ai.md) picks the answer that fits best.
   Claude takes a second or two; a local model can take much longer the first time.
5. Nothing fits, or speech-to-text is off: the call takes the **Not understood** line.

Every answer has its own port on the right of the step; draw a line from each to where the call should go.
A common set-up: connect **No input** to the speech question and set **Repeat menu** to 0, so callers who press
nothing are asked to say it, while callers who know the key still press it. The [call journey](call-journey.md)
shows what the caller's answer was recognised as.

!!! note "Requirements"
    Speech-to-text must be on under [Speech](prompts.md#speech) with a Whisper model downloaded. The *base* or
    *small* model keeps the wait short.

## Test call

Click **Test call**, enter your extension and click **Call me**. Your phone rings; answer it to hear the menu as a
caller would. Save the menu first.

## Use the menu

An IVR menu is a [destination](concepts.md#destinations). Use it in an inbound route, behind a time condition (open
› menu, closed › voicemail), or as a submenu of another menu.

!!! example "A complete main number"
    Inbound route `050 700 1234` › time condition *Office hours* › **open**: IVR *Main menu*
    (1 › Sales intro › ring group Sales, 2 › queue Support, 3 › Daan, no input › repeat), **closed**: voicemail.

!!! note "Older menus"
    Menus made before the canvas existed (list of keys) are converted to a flow automatically the first time you
    open them.
