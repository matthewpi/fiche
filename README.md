# fiche

Simple hastebin proxy server similar in nature to the original
[fiche](https://github.com/solusipse/fiche).

Instead of storing data ourselves, we rely on a [haste-server](https://github.com/toptal/haste-server)
or any HTTP server that accepts a `POST /documents` request that returns a JSON response in the
format of `{"key":"string"}`. The `key` is then joined to the server's URL, forming a response of
`{url}/{key}` (e.x. `https://ptero.co/{key}`) that is sent back to the caller.

This tool is used to allow easy sharing of logs and files without any complex bash scripting or
software installation on systems.

## Client-side Usage

Requests:

```shell script
echo 'Hello, world!' | nc ptero.co 99
```

```shell script
cat file.txt | nc ptero.co 99
```

Response:

```text
https://ptero.co/{key}
```

## Server-side Usage

```text
$ fiche --help
Usage: fiche [flags]

Flags:
  -h, --help                         Show context-sensitive help.
      --listen=":99"                 Listen address
      --hastebin=https://ptero.co    haste-server URL
```

## Building

```bash
CGO_ENABLED=0 go build -v -trimpath -o dist/fiche github.com/matthewpi/fiche
```
