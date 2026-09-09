import type { ActionKind } from '@/api/types';

type Step = [number, 'stdout' | 'stderr', string];

export const scripts: Record<ActionKind, { command: string; steps: Step[]; exitCode: number }> = {
  doctor: {
    command: 'ctdev doctor',
    exitCode: 0,
    steps: [
      [300, 'stdout', 'ctdev doctor 12.23.0 on ctpi01 (linux/arm64)'],
      [500, 'stdout', '  dns.resolvers      pass   Pi-hole on 127.0.0.1, fallback 9.9.9.9'],
      [400, 'stdout', '  dns.dnssec         pass   dnssec-failed.org returns SERVFAIL'],
      [900, 'stdout', '  dns.roots          pass   a.root-servers.net answered authoritatively'],
      [300, 'stdout', '  disk.pressure      warn   / at 81%'],
      [300, 'stdout', '  thermal            pass   52 °C'],
      [700, 'stdout', '  docker.containers  pass   6 running, 0 unhealthy'],
      [400, 'stdout', '  backup.timer       pass   last snapshot 6 hours ago'],
      [300, 'stdout', '  security.ssh       pass   password auth off'],
      [500, 'stdout', ''],
      [100, 'stdout', 'Disk is filling: / is at 81%. Run cleanup or grow the volume.'],
      [200, 'stdout', '8 checks: 7 pass, 1 warn'],
    ],
  },
  update: {
    command: 'ctdev update -y',
    exitCode: 0,
    steps: [
      [400, 'stdout', 'Checking apt, components and compose stacks...'],
      [1200, 'stdout', '  apt        3 packages'],
      [300, 'stdout', '  pihole     image update available (2026.09.1)'],
      [200, 'stdout', ''],
      [800, 'stdout', 'Installing 3 packages...'],
      [1500, 'stdout', '  libssl3 3.0.15 → 3.0.16'],
      [900, 'stdout', '  curl 8.5.0 → 8.5.1'],
      [700, 'stdout', '  ca-certificates 2026.08 → 2026.09'],
      [400, 'stdout', 'Pulling pihole/pihole:latest...'],
      [2500, 'stdout', '  pulled sha256:8b1f…3e2a'],
      [1200, 'stdout', '  pihole recreated'],
      [200, 'stdout', 'Done. No reboot required.'],
    ],
  },
  status: {
    command: 'ctdev status',
    exitCode: 0,
    steps: [
      [300, 'stdout', 'Nothing needs a reboot.'],
      [200, 'stdout', 'Disk: / at 81% (attention above 80%).'],
      [200, 'stdout', 'Containers: 6 running, 0 unhealthy.'],
      [200, 'stdout', 'Backups: last snapshot 6 hours ago.'],
      [200, 'stdout', 'Updates: 3 pending.'],
    ],
  },
  backup: {
    command: 'ctdev backup now',
    exitCode: 1,
    steps: [
      [400, 'stdout', 'Snapshotting 3 paths to b2:ctpi01...'],
      [1800, 'stdout', '  /home/ctadmin/pihole    12.4 MB'],
      [900, 'stdout', '  /etc                     1.1 MB'],
      [2200, 'stderr', 'Fatal: unable to open repository: connection reset by peer'],
      [100, 'stderr', 'restic exited 1'],
    ],
  },
};
