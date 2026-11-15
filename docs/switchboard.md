# Switchboard and wallboard

The **Reception** menu is for the reception and team leads (role *Reception* and up): the switchboard to handle
calls live, the wallboard for a screen on the wall, plus fax, call history and recordings.

## Switchboard

![The switchboard with day/night switch and live calls](shot:switchboard)

The switchboard shows, live:

- **Day / night**: every [time condition](time-conditions.md) with its switch code, set to *Automatic*, *Forced open*
  or *Forced closed* with one click.
- **My active calls**: the calls on your own extension, ready to transfer.
- **Live calls**: every call in progress, with both parties and the duration.
- **Queues**: waiting callers (with the longest wait), answered and abandoned calls, and every agent. Sign agents
  **in** or **out**, **pause** and **resume** them.
- **Colleagues**: the busy lamp field: available, ringing, in a call, on hold, do not disturb or offline.

### Transfer by drag and drop

Drag any live caller onto a colleague, ring group or queue to transfer the call there. For your own calls choose
**Transfer directly** (blind) or **Ask first** (attended): the target is called while the caller waits on hold; then
**Complete transfer** or go **Back to caller**.

No mouse? Press the **Transfer** button on a call and pick the target; <kbd>Esc</kbd> cancels. This also works on
touch screens.

### Supervision

Next to an agent in a queue call of a queue with [supervision](queues.md#supervision-listen-whisper-barge-in) on, the
buttons **Listen**, **Whisper** and **Barge in** appear. Your own phone rings; answer to join the call.

### Callbacks

The **Callbacks** list shows the callers who asked a queue to call them back, with their status: *Scheduled*,
*Waiting in queue*, *Calling back* (with the agent), *Called back*, *Not reached* or *Cancelled*. Cancel an open
callback, try a failed one again, or add a callback by hand. See [callback from the queue](queues.md#callback-from-the-queue).

## Wallboard

![The queue wallboard](shot:wallboard)

The wallboard shows the queues at a glance for the whole team: waiting now, answered and hung up today, the longest
wait, the service level, and every agent (free, in a call, paused). Click **Full screen** for a TV on the wall. It
updates by itself every few seconds.

!!! tip "A dedicated wallboard screen"
    Make a user with the role *Reception* for the screen, sign in once on the TV browser and open the wallboard in
    full screen.
