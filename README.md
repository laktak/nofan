
# nofan Framework 16

{ repos & mirrors: [github.com/laktak/nofan](https://github.com/laktak/nofan/), [codeberg.org/laktak/nofan](https://codeberg.org/laktak/nofan) }

This is a very opinionated fan controller for the Framework 16

Its goals are

- keep the fan off (when possible)
- when the fan is on, try to keep the speed (=noise) constant

This works well for my sway setup. YMMV.

At the moment there is no configuration. It could be use with other hardware by replacing `ectool`.

If you are looking for an alternative (which I took the ectool idea from), see https://github.com/TamtamHero/fw-fanctrl

## Setup

On arch, use PKGBUILD/AUR (todo)

On other systems with systemd:

```
# install binary
install -Dm755 nofan "/usr/bin/nofan"

# install systemd service
install -Dm644 systemd/nofan.service "/usr/lib/systemd/system/nofan.service"

# install tmpfiles configuration
install -Dm644 systemd/tmpfiles.conf "/usr/lib/tmpfiles.d/nofan.conf"

# install sleep configuration
install -Dm644 systemd/on_sleep "/usr/lib/systemd/system-sleep/nofan"
```

Dependencies:
- https://gitlab.howett.net/DHowett/ectool
