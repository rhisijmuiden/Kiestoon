# Call queues

A **call queue** lets callers wait in line, with music and position announcements, until an agent is free. Calls
are spread over the agents by a distribution strategy. Numbers are in the 700 range by default.

![Call queues with live waiting callers](shot:queues)

## Create a queue

1. Open **Call queues** and click **New queue**.
2. Pick a **distribution**:

    | Distribution | Who rings |
    | --- | --- |
    | **Round robin** | Agents take turns, starting after the last one who got a call |
    | **Ring all** | All free agents at once |
    | **Longest idle** | The agent who has been idle longest |
    | **Fewest calls** | The agent with the fewest calls today |
    | **Random** | A random free agent |
    | **In order** | Agents in a fixed order |
    | **Random by penalty** | Random, a higher penalty gives a lower chance |

3. Add **agents**. **Dynamic** agents sign in and out themselves (user portal, desktop softphone or `*45`); agents
   set to **Always on** are always signed in. A **penalty** puts an agent further back.
4. Optional: a **greeting** (a [voice prompt](prompts.md) played once when the caller enters), **music on hold**
   and **announce position** every N seconds. Callers hear the announcements in the
   [caller language](settings.md#caller-language); **Caller language** on the queue sets it apart.
5. Set the timings and limits, and **when the queue is full or too slow**: the overflow destination.

![Editing the Support queue](shot:queue-edit)

You can also change a queue, or create a new one, straight from the [IVR builder](ivr.md#queue-settings-in-the-editor): click a queue step on the canvas.

## Timings and limits

| Setting | Meaning |
| --- | --- |
| **Ring an agent for** | Seconds an agent rings before the next one is tried |
| **Wait before the next agent** | Pause between two attempts |
| **Wrap-up time** | Seconds after a call before the agent gets the next one |
| **Maximum wait** | After this the caller goes to the overflow destination |
| **Maximum callers** | More callers go straight to the overflow (0 is unlimited) |
| **Accept callers when no agent is signed in** | Off: callers go straight to the overflow destination |
| **Record queue calls** | Every answered call in this queue is recorded |

## Callback from the queue

Turn on **Queue callback** and waiting callers regularly hear that they can press **1** to be called back. They
confirm their number (or type another one followed by #) and hang up. They keep their place in the queue: a
placeholder waits for them. As soon as an agent is free, that agent's phone rings first (the display shows
`CB` and the caller), the agent hears a short message and Kiestoon calls the caller through the outbound routes.

| Setting | Meaning |
| --- | --- |
| **Offer every** | How often waiting callers hear the offer |
| **Extra attempts** | How many more times Kiestoon tries when the caller does not answer (a retry goes to the front of the queue) |
| **Time between attempts** | Wait before the next attempt |
| **Give up after** | Minutes after the request after which an unanswered callback stops |
| **Caller ID for callbacks** | The number the caller sees; empty uses the outbound route |

The open callbacks are on the [switchboard](switchboard.md#callbacks), where reception can cancel one, try again
or add a callback by hand (for example after a missed call).

!!! note
    Callers with a hidden number are asked to type their number. Callbacks that were waiting when Kiestoon or
    Asterisk restarts are placed back in the queue; a callback that was being dialled at that moment is marked
    *Not reached* and can be tried again.

## Supervision: listen, whisper, barge in

With **Supervision** on, users with the role *Reception* or higher who have their own extension can join queue
calls of that queue:

| Mode | Switchboard | Phone | Who hears the supervisor |
| --- | --- | --- | --- |
| **Listen** | ear button | `*33<extension>` | nobody |
| **Whisper** (coach) | speech button | `*34<extension>` | only the agent |
| **Barge in** | people button | `*35<extension>` | agent and caller |

On the switchboard the buttons appear next to an agent who is in a queue call; your own phone rings and you are
connected. During the call, press **4** to listen, **5** to whisper and **6** to barge in. Only queue calls can be
joined, never other calls of the agent, and every use is written to the audit log.

!!! warning "Privacy"
    Listening in on calls is processing of personal data. Tell your employees (and, where needed, callers) that
    calls in this queue can be listened to, for example in the greeting and in your privacy policy.

## Agents on other sites

With [linked systems](multi-site.md), agents of another Kiestoon can join the queue (**Agents on linked systems**).
Their phones ring through the tunnel and their busy lamp tells the queue when they are in a call.

## Following a queue live

- The **Call queues** page shows waiting callers and signed-in agents.
- The [wallboard](switchboard.md#wallboard) is a big-screen view for the team.
- The [switchboard](switchboard.md) lets reception sign agents in and out and pause them.
- Agents use **Queues & groups** in the [user portal](portal.md) or `*45` / `*45<queue>` on the phone.
