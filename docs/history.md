# Call history and recordings

## Call history

**Call history** (Reception menu) lists every call: time, caller, called number, direction, result, duration and
trunk. Search on a number, filter on date range, direction (inbound, outbound, internal) and result (answered, no
answer, busy, failed), and click **Export CSV** for a spreadsheet.

![Call history](shot:history)

Employees see their own calls in the user portal under **My calls**, with click-to-call to call someone back.

## Recordings

Calls are recorded when:

- an extension has recording **Always** on,
- an extension has recording **On request** and the user presses `*1` during the call (again to stop),
- a [queue](queues.md) has **Record queue calls** on,
- a [conference room](conferences.md) has **Record meetings** on.

**Recordings** (Reception menu) lists them with date, from, to and length. Listen in the browser, download or
delete. Employees find their own recordings in the user portal.

![Recordings](shot:recordings)

### Retention and the GDPR

Under **Call features & fraud › Recording**:

| Setting | |
| --- | --- |
| **Keep recordings (days)** | Older recordings are removed automatically. 0 keeps them forever; mind the GDPR (AVG) retention rules |
| **Beep during recordings** | Callers hear a short beep every 15 seconds |

!!! info "Tell callers you record"
    Under the GDPR you must tell callers that calls may be recorded and why. A line in the welcome prompt of your
    [IVR menu](ivr.md) or [queue](queues.md) is the usual way.
