# ssh sync

Watch a local folder, and sync any changes to a remote folder over ssh.
Sends the minimal delta over the ssh connection to minimize latency.
For this to work you have to have it installed on both the client and the server.

```
Options:

  -h, --help           display help information
      --addr          *server address
      --user[=$USER]   server username
      --port[=22]      server port
      --remote        *server path
      --local         *local path
```
