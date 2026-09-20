# AI capabilities

## What it is

ctdev, and the agent it becomes on each managed device, can reason about a situation it was not
programmed for. It works out what is wrong or what is needed, then either proposes what to do or
tells the user which ctdev commands to run: to set up a machine, to diagnose an issue, to fix one.

The reasoning is expected to be split between two kinds of model that work together with no human
between them. A large language model, probably Claude, handles the open-ended part: researching a
problem and writing a plan. A decision model handles the bounded part: it takes the state of a
device and returns a typed choice from a set defined in advance, with a confidence, in
milliseconds and at a fraction of the cost. TypeSafe AI calls this class a System One model, and
its Jev is the candidate. The aim of the split is a tool that is faster, smarter and cheaper than
one that sends everything to the large model.

This is an intention. Nothing is designed. Undecided: which models, where the seam between them
sits, what the tool may do on its own and what it may only suggest, and what runs on the device
and what runs centrally.

## Why ctdev wants it

Traditional software is a set of yes/no decisions baked in before release. A tool that sets up and
diagnoses devices meets situations nobody foresaw, and today each one waits for a new release that
adds a check or a fix for it. With AI capabilities ctdev can adjust to the situation in front of
it, in real time, without a release.

## In Linear

[Map: a spec for AI capabilities in ctdev and its agent](https://linear.app/conner-technology/issue/CON-70/map-a-spec-for-ai-capabilities-in-ctdev-and-its-agent),
in the [ctdev platform](https://linear.app/conner-technology/project/ctdev-platform-452e61b14666)
project. The undecided questions above are worked there, one ticket each.
