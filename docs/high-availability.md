# High availability

Two Kiestoon systems can run as an **active/passive pair**. The primary node runs the phone system; the standby node
copies its database and files continuously and takes over when the primary fails.

![Sites › High availability](shot:cluster)

## Set up

1. Install Kiestoon twice, with the **same version** (two LXC containers on different Proxmox hosts, or two machines).
2. On the first system (finish its first sign-in): **Sites → High availability**, choose the failover mode and click
   **Switch on high availability**. Kiestoon shows a join code.
3. Open the portal of the second system. Sign in with the user name and password of the second system, choose
   **Join an existing Kiestoon as standby node** and paste the code.

The second system restarts as standby. Its portal shows the replication state and a **Promote** form; everything else
is managed on the primary.

## Failover modes

**Virtual IP (same network).** Both nodes are in one network (layer 2). keepalived (VRRP, unicast between the nodes)
holds a virtual IP on the primary; phones and trunks use that address. When the primary fails, the standby takes the
virtual IP within a few seconds and promotes itself. Both nodes run keepalived in BACKUP mode with *nopreempt*, so a
recovered node does not take the address back by itself.

**By hand or by DNS.** The nodes are in different networks or data centres. Promote the standby from its own portal
(sign in with an administrator account of the primary) and point DNS (for example SRV records with a short TTL) or the
phones at it.

## What is replicated

- The database: the primary takes a consistent snapshot (`VACUUM INTO`) within seconds of every change; the standby
  fetches it through the tunnel.
- Files: the Asterisk spool (voicemail, recordings), prompts, fax, speech models, firmware and TLS certificates,
  compared by size and checksum every few seconds.
- Generated configuration (Asterisk, firewall, provisioning) is not copied: it is rebuilt from the database.

All replication runs through a dedicated WireGuard tunnel (`kt-ha`, UDP 51821) with a shared secret, so it also works
across the internet.

## Switch over

**Switch over** on the primary hands over cleanly: it takes a last snapshot, waits until the standby has it, then
steps down. The standby promotes itself; the old primary becomes the new standby. Use this for maintenance and
updates: update the standby, switch over, update the old primary.

## Split brain and data loss

- Calls in progress are dropped when the other node takes over. Registrations come back within the re-register
  interval of the phones.
- Changes of the last few seconds before a crash can be lost.
- When the standby is promoted while the primary still runs (network split), both nodes can run the phone system.
  The manual promotion refuses this while the primary still answers, unless *force* is checked.
- Virtual IP mode: when the nodes see each other again, keepalived keeps one master and the other node steps down to
  standby and copies from the master. Changes made on the node that stepped down during the split are lost.
- By hand or by DNS: an old primary that comes back still thinks it is the primary. Keep it off the network (or stop
  `kiestoon` and `asterisk` on it), then reinstall it and join it as the new standby.
- A node that is promoted keeps its own previous database in `/var/lib/kiestoon/cluster/previous`, for recovery by hand.

## Switch off

**Switch off** on the primary removes the pair. The standby hears it, forgets the pairing and becomes an empty system
again (setup wizard).

## Ports

| Port | Between the nodes | What |
| --- | --- | --- |
| UDP 51821 | both ways | WireGuard tunnel for replication |
| TCP 8443 | standby → primary, once | joining |
| VRRP (protocol 112) | both ways, virtual IP mode only | keepalived |
