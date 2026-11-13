# PROTOTYPE tape: Kanban Board on invented data, 120x32, dark.
# Run: go build -o /tmp/taiga-demo ./cmd/prototype-demo && vhs docs/media/demo.tape
Output docs/media/demo.gif
Set Shell "bash"
Set FontSize 16
Set Width 1400
Set Height 760
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms
Hide
Type "export TERM=xterm-256color; clear" Enter
Type "/tmp/taiga-demo" Enter
Sleep 2s
Show
Sleep 1s
Right 2
Sleep 700ms
Down 1
Sleep 700ms
Sleep 1200ms
Screenshot docs/media/hero.png
Sleep 1s
Type "s"
Sleep 1200ms
Screenshot docs/media/status.png
Escape
Sleep 1s
Hide
Type "q"
Sleep 500ms
