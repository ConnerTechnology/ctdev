# ctdev

The language of ctdev, Conner Technology's internal tool for managing and maintaining devices and
the networks they sit on. This file is a glossary and nothing else.

## Language

**ctdev**:
Conner Technology's internal tool for managing and maintaining devices and networks: personal ones
today, clients' later.
_Avoid_: dotfiles, bootstrap tool

**Device**:
Anything on a network that ctdev touches. The umbrella term; every other noun here is a kind of
device or the place a device sits.
_Avoid_: host, node (as the umbrella), endpoint

**Managed device**:
A device with ctdev installed on it, so it can report its health and take commands.
_Avoid_: client, agent (the agent is the software, not the device)

**Unmanaged device**:
A device ctdev can see and diagnose but cannot be installed on: an access point, a router, a
switch, a printer.
_Avoid_: gear, peripheral

**Machine**:
A managed device that is a general-purpose computer: a laptop, a desktop, a Pi, a server. Every
machine is a managed device; a phone running ctdev is a managed device but not a machine.
_Avoid_: box, computer, workstation (as the general term)

**Fleet**:
The set of devices one owner manages together with ctdev, with its own dashboard and users and no
view into any other fleet. A fleet spans many networks; a network is not a fleet.
_Avoid_: net, tailnet, tenant, org

**Network**:
The environment a device sits in, which ctdev sets up and diagnoses in its own right.
_Avoid_: LAN, site
