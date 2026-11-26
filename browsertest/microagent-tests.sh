#!/bin/sh
# microagent-tests.sh writes the browser test folders of the micro agents
# from the microagents folder of skycoin-cx-playground-android that go.mod
# pins: a replay test per agent and a codingbench test per agent and task of
# the scripted-stub run. Run it from browsertest/ after a bump of that pin.
set -eu
microagents=$(go list -m -f '{{.Dir}}' github.com/skycoin/skycoin-cx-playground-android/microagents)

for agent in femtoclaw mimiclaw zclaw; do
	dir=tests/microagent-$agent-replay
	mkdir -p "$dir"
	cat > "$dir/steps.txt" <<EOF
# Replays the $agent CX port twice from the tape of its replay-determinism
# test in microagents, on the wasm build of cx in a Worker. Replay needs no
# model and no network.
open /
wait-text #status Ready.
files \$microagents/$agent/port /home/cx/agent/port
files \$microagents/$agent/tests/replay-determinism/tape.txt /home/cx/run
spawn first /home/cx/run cx --enable=file,net,process /home/cx/agent/port ++mode=replay ++tape=tape.txt
spawn second /home/cx/run cx --enable=file,net,process /home/cx/agent/port ++mode=replay ++tape=tape.txt
EOF
	cat > "$dir/expect.txt" <<EOF
exit first 0
exit second 0
file-equals first.stdout \$microagents/$agent/tests/replay-determinism/expected-output.txt
file-equals second.stdout \$microagents/$agent/tests/replay-determinism/expected-output.txt
EOF
done

# zclaw has no file tools, so codingbench runs femtoclaw and mimiclaw only.
for run in "$microagents"/codingbench/runs/scripted-stub/*/*/; do
	task=$(basename "$run")
	agent=$(basename "$(dirname "$run")")
	dir=tests/codingbench-$agent-$task
	mkdir -p "$dir"
	files=""
	if [ -d "$microagents/codingbench/tasks/$task/files" ]; then
		files="files \$microagents/codingbench/tasks/$task/files /home/cx/run/spiffs"
	fi
	cat > "$dir/steps.txt" <<EOF
# Replays the scripted-stub tape of codingbench task $task with the $agent
# CX port in the browser, with ++apply=1, which writes the files the agent
# wrote. The files come back to the host, and the task's check.sh judges
# them with the native cx, as codingbench does.
open /
wait-text #status Ready.
eval jsfs.mkdirp("/home/cx/run/spiffs/sessions")
$files
files \$microagents/$agent/port /home/cx/agent/port
files \$microagents/codingbench/runs/scripted-stub/$agent/$task/tape.txt /home/cx/run
spawn replay /home/cx/run cx --enable=file,net,process /home/cx/agent/port ++mode=replay ++tape=tape.txt ++apply=1
pull /home/cx/run/spiffs spiffs
host check spiffs sh \$microagents/codingbench/tasks/$task/check.sh
EOF
	cat > "$dir/expect.txt" <<EOF
exit replay 0
file-equals replay.stdout \$microagents/codingbench/runs/scripted-stub/$agent/$task/output.txt
exit check 0
contains check.stdout solved
EOF
done
