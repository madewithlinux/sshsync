will use
* channel over network api
* diff: "github.com/sergi/go-diff/diffmatchpatch"
    * must keep previous change in memory
* inotify: "github.com/fsnotify/fsnotify"
    * will need to implement recursive watcher
* gzip compression (or maybe something faster?)
* use protocol buffers to minimize size of transfer
* ssh: "crypto/ssh"

skeleton:
* server
    * give index of files (map of paths to hashes?)
    * download all files (tarball? gzipped protocol buffer?)
    * download list of specific files
    * accept and apply file edits
* client
    * connect to server via ssh and start server
    * retrieve index from server and update what is out of date
        * remove files not on server?
    * suck all files into ram
    * listen for edits
    * on edit, diff file with copy in ram and send patch to server
        * (patch is compressed)
        * wait for server to acknowledge and print message?


## network format

### Diff
```
diff
<number of files diffed>
<files[0] filename>
<files[0] delta>
...
<files[N] filename>
<files[N] delta>
```