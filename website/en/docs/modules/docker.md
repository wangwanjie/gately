# Docker

Manage containers, Compose projects, images, networks and volumes on the host through the Docker socket. The entry point is "Docker" in the sidebar, and the page is split into six tabs: "Containers", "Compose", "Images", "Networks", "Volumes" and "System". When Docker cannot be reached, the top of the page shows the error details and a shortcut button to open the settings.

## Enabling

When Gately runs in a container, mount the host's Docker socket:

```bash
-v /var/run/docker.sock:/var/run/docker.sock
```

The Docker address defaults to `unix:///var/run/docker.sock` and can be changed in the settings on the "System" tab.

::: warning Security note
Mounting `docker.sock` is equivalent to granting root-level access to the host. If you do not need Docker management, remove the mount and protect the management UI (two-factor authentication, security entrance, LAN-only access).
:::

## Containers

- The list can be filtered by "All", "Running" and "Stopped". Cards show the image, network, ports, mounts and update status;
- Actions: start, stop, restart, pause, resume, force kill, rename, delete (optionally with anonymous volumes), commit as image;
- **Self-protection**: Gately recognizes its own container and Compose project, and rejects operations that would cut off access to itself: stopping, force-killing, pausing or deleting its container, and running `down` on its project. Edit-and-recreate, and running `up` or deleting its project, all have to stop the old container first; the Gately process would exit along with it and could not finish the recreation, so these are rejected too, and the corresponding buttons are grayed out in the interface. The protection also applies when the API is called with a container name or ID prefix.
- **Upgrading Gately itself**: when you upgrade Gately's own container, Gately pulls the new image first and, only if there is a new version, creates a temporary helper container from its current image (`<container name>-upgrade-<time>`). The helper stops the old container, creates and starts the new one with the same configuration and removes the old one, restoring the old container if the new one fails to start. Meanwhile the interface shows a waiting screen and reloads once Gately is back; the new Gately writes the helper's output to the Docker log and removes the helper. Batch upgrades handle Gately last and leave it alone when another container failed. The helper reaches the Docker socket through the same mount as Gately (such as `/var/run/docker.sock:/var/run/docker.sock`); without that mount the upgrade is rejected. Do not recreate Gately with `docker compose up -d`, `docker restart` and similar commands in the local shell of the Web Terminal: in a Docker deployment that shell runs inside the Gately container, so the command is killed together with Gately and leaves the stopped old container next to a new one with a temporary name (`<ID prefix>_<container name>`). If that happens, run `docker compose up -d` once more in the Compose directory on the host to recover. Upgrading on the host with `docker compose pull && docker compose up -d` keeps working as before.
- **Sensitive information**: when a read-only account or an API token with only `docker:read` views a container, environment variables show only their names, and the values, Command, Entrypoint, startup arguments and the command arguments in the process list are shown as `********`. Image details likewise hide environment variables, Cmd, Entrypoint and build history commands (committing a container as an image writes these settings into the image as they are). Administrators with write access get the plain text automatically when they open the edit form.
- **Create and edit**: the form covers name, image (can be pulled before creation), start after creation, command and Entrypoint, environment variables, port mappings (TCP/UDP, with an optional host IP), mounts (directory bind and volume, optionally read-only), primary network and extra networks, fixed IP, aliases, restart policy (`no`, `always`, `unless-stopped`, `on-failure` with a maximum retry count), privileged mode, adding or dropping capabilities, device mappings, user, working directory, hostname, DNS, labels, memory limit (MB), CPU count, TTY and keep stdin open. Editing recreates the container based on its original configuration; settings the form does not cover keep their original values, and a failed recreation rolls back automatically;
- Detail page tabs:
  - Overview: status, exit code, restart count, health status, network details, mounts, and the Compose project and service it belongs to;
  - Logs: view recent logs, with optional timestamps;
  - Terminal: open an interactive terminal inside a running container (`/bin/sh` by default; command and user can be specified; colors follow the Web terminal preference);
  - Files: browse files in the container, edit text online (up to 1 MiB), upload, download (directories are packed as tar.gz), rename, create directories, delete. Deletion runs `rm -rf` inside the container, so the container must be running;
  - Monitoring: samples of CPU, memory, network and disk I/O, and process count, plus the list of processes inside the container.

## Compose

Compose projects are aggregated from container labels such as `com.docker.compose.project`. They show a status (running, partially running, stopped) and their services.

- Start, stop, restart, pause, resume, `down` (optionally with named volumes), and view combined logs (each line prefixed with the service name);
- Create a project: paste the compose content, and Gately writes it to `compose/<name>/compose.yaml` in the configuration directory and runs up. Such projects are marked "managed"; you can edit the Compose file, save and apply it, or delete the whole project;
- For existing projects, the first compose file can be edited online (the file must be writable), with an option to run up immediately after writing it back;
- `up`, image pulls, builds and file reads and writes depend on the `docker compose` command line and access to the compose file path. When unavailable, the UI shows "docker compose command not found".

## Images

- The list shows tags, size, creation time, the number of containers referencing each image and dangling status. Details include layer history, environment variables, exposed ports and more;
- **Pull**: authenticates with the registry accounts in the "System" settings. If a direct pull fails and mirror addresses are configured, they are tried in order and the image is re-tagged with its original name. Docker Hub search is supported;
- Push, add tag, delete (optionally forced), build (inline Dockerfile or Git URL, with build arguments), and import and export tar (`docker save` format);
- **Update check**: check a single container or use "Check all updates"; results are marked on each container;
- **One-click upgrade**: pulls the new image and recreates the container with its existing configuration; a failed upgrade rolls back to the original container automatically. Multi-select for batch upgrades is supported;
- "Clean dangling" removes all untagged, unused images in one click.

## Networks and volumes

- Networks: the list shows driver, scope, subnet, gateway and attached containers. You can create (driver, subnet, gateway, internal network, IPv6) and delete networks, and connect or disconnect containers. Built-in networks are protected;
- Volumes: shows driver, mount point, size, reference count and the containers using each volume. You can create, delete (optionally forced), export as tar.gz, and import from tar.gz. Export and import use a temporary alpine helper container, and the image is pulled automatically if missing.

## System

The "System" tab contains:

- Host and Docker info: version, API version, OS and architecture, kernel, CPU and memory, storage driver, data directory, container and image counts, Compose CLI version;
- Disk usage: count, size and reclaimable space for images, containers, volumes and build cache;
- Cleanup: stopped containers, dangling images, all unused images, unused networks, unused volumes (data is lost permanently), build cache;
- Settings:

| Field | Description |
| --- | --- |
| `host` | Docker address, default `unix:///var/run/docker.sock` |
| `mirrors` | List of mirror addresses, tried in order when a direct pull fails. Independent of `registry-mirrors` in `/etc/docker/daemon.json`, which Docker uses automatically and the settings page shows read-only as "Daemon mirrors" |
| `registries` | Private registry credentials (`server`, `username`, `password`); the password is masked in the UI |
| `autoCheckHours` | When greater than 0, container image updates are checked automatically at this interval in hours; 0 turns it off; newer images publish a `docker.update` event, once per new image |

## Background tasks

Long-running operations such as pull, push, build, batch check, batch upgrade and Compose up run as background tasks. The task panel at the bottom of the page shows live output and status (running, done, error, stopped), lets you view history, and clears finished tasks. Tasks that are still running are restored after a page refresh.

A running task can be stopped with "Stop" in the task panel or under "System → Task history". After you confirm:

- Pulling, pushing, building images and checking for updates stop immediately, and Docker also aborts the corresponding download or build, so no half-pulled image is left behind;
- Compose commands first receive an interrupt signal (the same as pressing Ctrl+C in a terminal) and wrap up on their own; they are killed only if they have not exited within 30 seconds;
- A batch upgrade stops once the current container has been handled, and the remaining containers are not upgraded. A container that is being recreated (stopping the old container, creating and starting the new one) ignores the interruption and finishes the recreation; if that fails, the original container is restored as usual, so nothing stops halfway.

After an interruption the task status is "Stopped" and the run log records "Task stopped: …"; a task that had already finished when you interrupted keeps its actual result. A stuck task can be cleaned up with "Clear finished" once it is stopped. The matching API is `POST /api/docker/tasks/{id}/cancel`.

## Event notifications

Gately subscribes to the Docker event stream and publishes a `docker.exited` event when a container exits abnormally (non-zero exit code) or is killed by the OOM killer. You can configure receiving channels in the [Notification Center](/en/docs/modules/notify). A normal stop with exit code 0 does not trigger a notification. When Docker is temporarily unavailable, the subscription is retried every 30 seconds.

## API

See the [Docker API](/en/docs/reference/docker-api) for the complete API reference.
