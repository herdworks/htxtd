# htxtd

`htxtd` is a static website server.

```sh
htxtd --bind 0.0.0.0 --port 8080 builds/development
```

The directory argument is required and is resolved from the current working directory. 
The default bind address is `localhost`; the default port is `8080`.

The `htxtd.Server` package API can also provide server-owned browser live reload. 
Set `LiveReload`, call `Reload` after a successful external rebuild, and the server injects its client into full HTML responses without changing the files on disk.
