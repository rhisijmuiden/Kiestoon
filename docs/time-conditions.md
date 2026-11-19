# Time conditions and holidays

A **time condition** sends calls one way during opening hours and another way outside them, with a separate
destination for holidays and a day/night switch for the unexpected.

![Time conditions with their current state](shot:time-conditions)

## Create a time condition

1. Open **Time conditions** and click **New time condition**.
2. Give it a name, for example *Office hours*.
3. Add the **opening hours**: days and a from/until time. A lunch break is a second range (for example 08:30-12:00
   and 13:00-17:30). Times use the time zone in [Settings](settings.md).
4. Choose the destinations: **When open**, **When closed** and optionally **On holidays** (empty: same as closed).
5. Keep **Closed on holidays** on to use the holiday calendar.

![Editing the Office hours time condition](shot:time-condition-edit)

## Day/night switch

Every time condition has a **mode**:

| Mode | Effect |
| --- | --- |
| **Automatic** | Follows the opening hours and holidays |
| **Forced open** | Always open, for example when you stay open late |
| **Forced closed** | Always closed, for example during an emergency or a company outing |

Switch it on the Time conditions page, on the switchboard (reception), or with the **switch code** `*28<id>` from
any phone; the code is shown on the time condition. Dialing the code again returns to automatic.

## Holidays

The **Holidays** tab holds the calendar used by all time conditions.

![The holiday calendar](shot:holidays)

- **Dutch holidays**: New Year's Day, Good Friday, Easter, King's Day, Liberation Day, Ascension Day, Whitsun,
  Christmas and Boxing Day. Dates move with Easter and are calculated every year. Switch off the ones you work on.
- **Own closing days**: for example a company outing or the days between Christmas and New Year, once or **every
  year**.

## Using a time condition

A time condition is a [destination](concepts.md#destinations): put it behind an inbound route, inside an
[IVR menu](ivr.md) (a time condition step with *open* and *closed* lines) or in another time condition.
Employees can also use a time condition as their personal schedule in the user portal (forward when closed).
