# UniFi network setup

## What it is

ctdev sets up a UniFi network to a standard, the way it sets up a machine from a profile.

It covers UniFi only. A network built on any other vendor's devices gets diagnosis only.

Two things are undecided: the standard itself, which Thomas will define from the home network, and
the mechanism ctdev uses to configure the UniFi devices.

## Why ctdev wants it

ctdev is for devices and the networks they sit on. For a network it can do two things: diagnose
it, and build the machines that serve it. It does not configure the unmanaged devices a network is
made of: the router, the switches, the access points. This idea closes that gap for UniFi, so that
a network is set up in a common way just as a machine is.
