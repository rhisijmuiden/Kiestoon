# Conference rooms

A **conference room** is a number that several people dial at the same time, from inside or outside (route a phone
number to it). Numbers are in the 800 range by default.

![Conference rooms](shot:conferences)

## Create a conference room

1. Open **Conference rooms** and click **New conference room**.
2. Set a **PIN** (empty: anyone who reaches the number can join) and a **host PIN**. The host PIN gives host rights:
   mute participants and lock the room.
3. **Maximum participants** (default 20).
4. **Wait for the host**: participants hear music until someone joins with the host PIN.
5. **Record meetings**: the whole meeting is recorded and shows up under [Recordings](history.md#recordings).

To let outside callers join, add an [inbound route](routes.md) with the conference room as destination, or a key in
an [IVR menu](ivr.md).
