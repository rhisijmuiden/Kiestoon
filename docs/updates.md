# Updates and channels

Kiestoon updates itself from the portal. Every release is published on
[GitHub](https://github.com/rhisijmuiden/Kiestoon/releases) with its `.deb` packages, LXC templates, the Windows
softphone and a `SHA256SUMS` file.

## Info & updates

The **Info & updates** button at the bottom of the menu shows the version, commit, build date, host name, uptime and
update channel.

![Info & updates](shot:info)

1. Click **Check for updates**. Kiestoon asks GitHub for the newest release on your channel.
2. When a newer version is available, read the **Release notes** and click **Update now**.
3. Kiestoon makes a backup, installs the new version and restarts. The portal is unavailable for about a minute and
   comes back by itself. Calls in progress are dropped, so update outside office hours.

Only administrators can change the channel or update.

## Channels

| Channel | What you get |
| --- | --- |
| **Production** | Stable releases (`v0.6.0`). Recommended. |
| **Testing** | Release candidates (`v0.6.0-rc.12`), to try new features before everyone else. |

The channel is switched in the same window. Going from Testing back to Production can mean that the newest
production version is older than the one you run; Kiestoon then says so and lets you install it.

The menu always shows the channel with a coloured dot: green for production, orange for testing.

## What happens during an update

The update runs as a separate system service (`kiestoon-updater`), so it survives the restart of the portal:

1. It downloads the packages and `SHA256SUMS` from the GitHub release and checks the checksums.
2. It stops the portal and backs up the data directory to `/var/lib/kiestoon-backups` (the newest 5 are kept).
3. It installs the packages, which restarts the portal and applies the new configuration.

When something fails, the window shows the error and the backup it made. Updates do not replace your
[backups to a remote location](backups.md); keep those running as well.

## Updating by hand

On the command line, install a downloaded package with `apt install ./kiestoon_<version>_amd64.deb`. For
[branch office edges](edge.md) that is the way to update.

## Updating a high-availability pair

Update the standby first, then click **Switch over**, then update the old primary. See
[High availability](high-availability.md#switch-over).
