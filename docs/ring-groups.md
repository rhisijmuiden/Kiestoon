# Ring groups

A **ring group** rings a team on one number, for example sales or the warehouse. Numbers are in the 600 range by
default.

![Ring groups](shot:ring-groups)

## Create a ring group

1. Open **Ring groups** and click **New ring group**. The next free number is filled in.
2. Choose the **ring strategy**:

    | Strategy | What happens |
    | --- | --- |
    | **All at once** | Every member rings at the same time. The first to answer gets the call |
    | **One by one** | Members ring one after another, in the order of the list |
    | **Rotating** | Like one by one, but each call starts at the next member |
    | **Intercom (paging)** | Phones answer automatically on the speaker. Only for extensions; good for announcements |

3. Add **members**: extensions, and with **Add an external number** also a mobile phone (an external member costs
   an outgoing call).
4. **Ring time**: how long the group rings (for *one by one* and *rotating*: per member).
5. **Caller ID prefix**: shown before the caller's name, for example `Sales: `.
6. **Skip busy members**: members who are already on a call do not ring.
7. **When nobody answers**: any [destination](concepts.md#destinations), for example a colleague's voicemail or a
   queue.

![Editing a ring group](shot:ring-group-edit)

## Ring group or queue?

| | Ring group | Call queue |
| --- | --- | --- |
| Callers wait | No: when nobody answers, the call goes to the failover | Yes, with music and position announcements |
| Members | Always on | Agents sign in and out (or always on) |
| Statistics and wallboard | No | Yes |
| Best for | Small teams, "ring everyone" | Support desks, busy numbers |

Employees see their ring groups and the status of the members in the user portal under **Queues & groups**.
