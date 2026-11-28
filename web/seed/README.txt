CX workstation

This is a terminal, a file manager, an editor and a browser in one browser
tab. The files live in the tab and are kept in the browser between visits.
cx is the real cx compiler and runtime, the same as the native cx. It runs
in a Worker, so Ctrl+C stops a program that never ends.

  cd demo && cx project                 a program of several files and packages
  cx project/main.cx project/greeting/*.cx
  seq 3 | cx echo.cx                    stdin and pipes
  cx --enable=file files.cx; cat notes.txt
  cx build --target=image -o fib.cximage ~/examples/fibonacci-recursive.cx
  cx-core-image fib.cximage             run the image cx build made
  cx --enable=net http-server.cx &      an HTTP server on 127.0.0.1:8080
  cx --enable=net http-client.cx        a CX client for it
  browse 127.0.0.1:8080                 the browser window opens it too
  edit FILE                             edit in this terminal, Ctrl+S saves, Ctrl+Q quits
  open FILE                             edit in a window of its own
  project-save DIR                      download a folder as a zip file
  project-load [--folder] [DIR]         upload a zip file, files or a folder
  zip, unzip, download, upload          the same, file by file
  help                                  every command of the shell

The examples folder holds the examples of the playground. The quick
single file playground is index.html beside this page.
