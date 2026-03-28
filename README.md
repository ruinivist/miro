# mire

E2E tests for CLIs.

Problem: There isn't an easy way to test E2E behavior for CLIs that is as simple as recording a set of actions and ensuring future builds replicate the output. What is usually done is code-based, which doesn't convey visually what the test actually is.

> _mire tries to make it as simple as recording and replaying_

![demo](demo.gif)

**How it works?**

Idea is that mire spawns a session and drops you in it to record your actions, as you would do for manual testing.\
Once done, it'll save the inputs and outputs as goldens to compare against.\
Tests will use those inputs later to recreate and verify that the output for the same set of input keystrokes is still the same.

**Features**

- sandboxed
- fast, explicit, simple, enough that a 10s gif can demonstrate it entirely
- record, test, that's it
- start simple, tweak the entire environment if you want

## Quickstart

Linux-focused for now; requires `bash` and `bwrap` to be available in `PATH`.

**Install**

- clone
- `make build`
- add `build/mire` to your `PATH`

**Using**

- `mire init` to create the single config file; every entry is explicit.
- `mire record test/name/` - now test how you would test manually, try out commands to see if they work as expected
- `mire test` or `mire test specific/test`
- `mire rewrite` - to rewrite all golden outputs in case of a style change

**Fixtures?**

This is for for modifying anything within the sandox.

You can write your script commands in `setup.sh` at any level. Anything at that and nested levels will be run before dropping you into record.

**Configuration**

There's a root level config file - `mire.toml` created via `mire init`.\
This is primarily for host to sandbox setup.

```toml
[mire]
  # which folder to strore tests in
  test_dir = "e2e"
  # regexes for differing lines to ignore during replay comparison
  ignore_diffs = []

[sandbox]
  # home is where mire would drop you by default on record
  home = "/home/test"
  # read only paths from host, entry looks like "path on host:path on sanbox"
  # paths on host are absolute or relative to repo root
  mounts = []
  # read only host paths to expose on PATH inside the sandbox as /tmp/mire/bin/<basename>
  # paths on host can be absolute or relative to repo root
  paths = []
```

All possible options are in this config, if it's not there isn't not possible at the moment.

For anything esoteric, you can always just modify the generated `shell.sh` directly as well.

> This branched off as a setup for another project so I've only added what I felt needed. Feel free to make an issue!

## Other tools?

VHS comes close, which is what I initially tried. It isn't suitable for testing because the output is terminal sessions captured as full frames in text. This makes it hard to know what the test is unless you watch it, and even then it has timing issues, multiple blank frames polluting goldens, and slow execution unless
you post-process to limit all the sleeps. It also requires you to handle the test environment and sandboxing yourself.
