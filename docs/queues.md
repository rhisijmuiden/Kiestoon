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
   and **announce position** every N seconds.
5. Set the timings and limits, and **when the queue is full or too slow**: the overflow destination.

![Editing the Support queue](shot:queue-edit)

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

## Agents on other sites

With [linked systems](multi-site.md), agents of another Kiestoon can join the queue (**Agents on linked systems**).
Their phones ring through the tunnel and their busy lamp tells the queue when they are in a call.

## Following a queue live

- The **Call queues** page shows waiting callers and signed-in agents.
- The [wallboard](switchboard.md#wallboard) is a big-screen view for the team.
- The [switchboard](switchboard.md) lets reception sign agents in and out and pause them.
- Agents use **Queues & groups** in the [user portal](portal.md) or `*45` / `*45<queue>` on the phone.
