# Wallboard, SLA alerts and reports

The wallboard shows your queues in large print for a screen on the wall: who is waiting, how long, today's service
level and what every agent is doing. Kiestoon watches the thresholds you set per queue and warns the team when a
queue gets into trouble.

![The wallboard on a TV](shot:wallboard-tv)

## What the wallboard shows

Each queue gets a tile with:

- **Waiting**: callers in the queue now, including callbacks (shown in purple, with the time since they asked).
- **Longest wait**: the caller who has waited longest. It turns red above the queue's maximum wait.
- **Service level**: the ring shows the share of today's calls answered within the target time. Green at or above
  the norm, orange up to 10 points below it, red below that.
- The callers in order, and at the bottom today's answered calls, hung up calls, average wait and average call.

The **Agents** panel lists everyone signed in to the queues, with a timer since their last change: in a call (with
the queue and caller), ringing, internal call, free (with today's number of calls), paused or offline. Below the
queues a chart shows the calls answered and hung up per hour today.

!!! info "How the service level is counted"
    Answered within the target time, divided by answered plus hung up calls. Callers who hang up within 5 seconds
    don't count (they most likely dialled the wrong number), nor do callers who left to the overflow or asked to be
    called back. The figures start at midnight and are kept for 400 days for reports.

## Layouts

The wallboard has three layouts:

| Layout | Shows |
| --- | --- |
| **Overview** | All queues, the agents and the calls per hour |
| **One queue large** | One queue at a time with its agents; with several queues it steps through them |
| **Agents** | A card per agent with state and timer |

![The agents layout](shot:wallboard-agents)

In the portal you choose the layout at the top of **Reception › Wallboard**, or **Rotate** to switch every 30
seconds. Supervisors with their own extension see the listen, whisper and barge buttons next to an agent in a call
(for queues with supervision on). On a TV link the layouts and the switch interval are part of the link.

## TV links

A TV does not sign in. As an administrator, open **Reception › Wallboard › Settings** and click **New TV link**. You
choose:

- a **name**, for example *Support room*;
- the **queues** to show (or all queues);
- the **layouts** and how often to **switch** between them;
- the **theme**, dark or light (light works better in rooms with a lot of daylight);
- **Hide caller names**: shows only the last 4 digits of numbers, for a screen visitors can see.

![Wallboard settings](shot:wallboard-settings)

Copy the link and open it on the TV, a Chromecast, a Fire TV stick or a mini PC with a browser, in full screen. The
link looks like `https://pbx.example.com/wall/k7Q2m-X9vLr-3e8sW-dPz4a`. The key in it is the secret: anyone with the
link sees the wallboard (and nothing else). **New key** replaces the key, so the old link stops working; **Revoke**
removes the link. *Last shown* tells you when a screen last fetched the figures.

![A TV link in the light theme, one queue large with names hidden](shot:wallboard-tv-light)

!!! tip "Keep the screen on"
    The TV page asks the browser to keep the screen awake. Set the TV or stick itself not to go to sleep as well.

## Design your own screen

Every TV link can get a design of its own in the **wallboard designer**: go to *Wallboard → Settings* and click **Design** on a screen.

**Layout.** Drag tiles from the list on the left onto the grid (12 × 8 for a landscape screen, 6 × 12 for portrait), or click a tile to place it on the first free spot. Drag a tile to move it and pull its corner to resize it; with the keyboard, the arrow keys move the selected tile and Shift + arrows resize it. A screen can have several pages that rotate, each with its own time. Undo and redo keep the last 50 steps.

There are 17 tiles: a number (KPI), a service level gauge, a trend, a queue tile, waiting callers, compare queues, callback requests, an agent list, agent tiles, a leaderboard, calls per hour, answered / abandoned, a clock, a text, a news ticker, a logo or image, and SLA alerts.

**Per tile** you choose the title, the figure (live: waiting, longest wait, callbacks, agents free / on a call / paused; or over a period: offered, answered, abandoned, abandoned %, service level, average wait and talk time, longest wait), which queues it counts, the period (today, last hour, yesterday, last 7 days), and when it turns orange or red. *Take over from the queue SLA* uses the thresholds of the queue (see below). A red figure can blink, and a figure of today can show yesterday's value next to it.

**House style & display.** Per screen: dark, light or automatic theme (light from 07:00 to 19:00), an accent colour, the Kiestoon logo or your own logo, background, tile style, text size, density, what the header shows, the language of the screen, and what happens on an SLA alarm (red banner, a sound on the TV, jump to the page with the alarm). Start quickly from a template: overview, one queue large, agents, reception compact, contact centre KPI or blank. *Powered by RHIS* always stays visible.

**Draft and publish.** *Save* keeps your work as a draft. The TV only shows it after **Publish**. *Preview* shows the draft full screen with live figures. A screen without a published design keeps showing the classic layouts.

## A screen's own address

Besides the long secret link, every screen has an address you choose yourself, for example `https://pbx.example.com/wall/reception`. Set it under *Address & access* in the designer, and choose who may open it:

- **Everyone who knows the address**: handy on your own network.
- **With a pin code**: the TV asks for the pin code (4 to 10 digits) once and remembers it. *New key* logs out all TVs.
- **Nobody**: the address is off; only the secret link works (the default for existing screens).

Under *Only from these networks* you can limit a screen to networks such as `192.168.10.0/24`. That applies to both the own address and the secret link. The screen only ever receives figures of the queues you chose for it, and wrong addresses or pin codes count towards the same block as wrong provisioning requests.

## SLA thresholds

Under **SLA thresholds per queue** you set per queue:

| Threshold | Alert when |
| --- | --- |
| **Target time** | Not an alert itself: the time within which a call counts as answered on time (default 20 s) |
| **Service level norm** | Today's service level is under this percentage (judged from 5 calls) |
| **Max. waiting** | More callers are waiting than this |
| **Max. wait** | A caller has waited longer than this |
| **Max. hung up** | More than this share of today's callers hung up (judged from 10 calls) |

Leave a threshold empty to not monitor it. The **Now** column shows whether the queue is within its thresholds.

## Alerts

Kiestoon checks the thresholds every 5 seconds. Under **Alerts** you choose where an alert goes:

- **On the wallboard and the switchboard**: a red banner at the top and a red border around the queue.
- **By e-mail** to the recipients, through the [mail server](mail.md). The **quiet period** (default 15 minutes)
  keeps the same alert of the same queue from being mailed more than once in that time.
- **Only mail** during the opening hours of a [time condition](time-conditions.md), so nobody gets mail at night.
- **Also mail when it is fine again** sends a short mail when the queue is back within the threshold.

Push notifications in the mobile app follow with the app.

## Scheduled reports

Under **Scheduled reports** you make an overview that Kiestoon mails by itself. Click **New report** and choose:

| Period | Sent | Covers | Compared with |
| --- | --- | --- | --- |
| **Daily** | every day (or only workdays) at the time you choose | that day | the same day a week earlier |
| **Weekly** | on the day and time you choose | the previous week, Monday to Sunday | the week before |
| **Monthly** | on the 1st at the time you choose | the previous month | the month before |

Choose the queues, the content (per queue, per agent, per hour, and the change against the period before), the
recipients and the language (English or Dutch) of the mail.

![Preview of a weekly report](shot:report-preview)

The mail is a plain HTML mail that works in every mail program. At the top it shows the calls, service level,
average wait and hung up share, each with the change against the period before. Below that the tables per queue,
per agent and per hour, the busiest hour and the number of SLA alerts. The figures per day and queue and per agent
are attached as a CSV file, which opens in Excel.

- **Preview** shows the mail with the current figures.
- **Send now to me** mails it to your own address (set it under [Users](users.md)).
- A report that cannot be delivered is not retried every minute: it waits for its next send moment. The
  [mail log](mail.md) shows what went wrong.
