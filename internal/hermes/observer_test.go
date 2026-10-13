package hermes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestObserverNativeHooksReloadAndPrivacy(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path, err := installObserver(filepath.Join(root, "plugin"))
	if err != nil {
		t.Fatal(err)
	}
	script := `import importlib.util
import json
import os
import pathlib
import sys
import time
import types

root = pathlib.Path(sys.argv[2])
os.environ['HOME'] = str(root)
os.environ['XDG_CACHE_HOME'] = str(root / 'cache')
sys.modules['psutil'] = types.SimpleNamespace(
    Process=lambda pid: types.SimpleNamespace(create_time=lambda: time.time()),
)
sys.modules['hermes_constants'] = types.SimpleNamespace(get_hermes_home=lambda: root)
spec = importlib.util.spec_from_file_location('observer', sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Context:
    def __init__(self):
        self.hooks = {}
        self.close = None

    def register_hook(self, name, callback):
        self.hooks[name] = callback

    def on_unload(self, callback):
        self.close = callback

    def emit(self, name, **data):
        self.hooks[name](**data)


ctx = Context()
module.register(ctx)
ctx.emit('on_session_start', session_id='one', platform='cli')
ctx.emit('pre_approval_request', session_key='one')
cache = root / ('Library/Caches' if sys.platform == 'darwin' else 'cache') / 'a-gent/hermes'
status = cache / 'instances' / (str(os.getpid()) + '.json')
preview = cache / 'previews' / (str(os.getpid()) + '.json')
assert json.loads(status.read_text())['state'] == 'waiting'
ctx.emit('post_approval_response', session_key='one')
assert json.loads(status.read_text())['state'] == 'running'

ctx.emit(
    'post_tool_call',
    session_id='one',
    tool_name='patch',
    status='error',
    result={'success': False, 'diff': 'FAILED', 'files_modified': ['bad']},
)
assert json.loads(preview.read_text())['edit'] is None

ctx.emit(
    'post_tool_call',
    session_id='one',
    tool_name='patch',
    status='ok',
    result={'success': True, 'diff': '-old\n+new', 'files_modified': ['file']},
)

ctx.emit('on_stream_delta', session_id='one', turn_id='turn', iteration=1, kind='text', delta='STREAMED')
assert json.loads(preview.read_text())['activity']['text'] == 'STREAMED'
ctx.emit('on_stream_delta', session_id='one', turn_id='turn', iteration=1, kind='reasoning', delta='PRIVATE')
assert json.loads(preview.read_text())['activity']['text'] == 'STREAMED'

ctx.emit(
    'post_llm_call',
    session_id='one',
    turn_id='turn',
    assistant_response='PUBLIC',
    conversation_history=[{'reasoning': 'PRIVATE'}],
)
ctx.emit('on_stream_delta', session_id='one', turn_id='turn', iteration=1, kind='text', delta='LATE')
ctx.emit('post_llm_call', session_id='child', assistant_response='OTHER')
ctx.emit('on_session_end', session_id='one')
ctx.emit('pre_llm_call', platform='subagent')
ctx.emit('post_llm_call', platform='subagent', assistant_response='UNATTRIBUTED')
ctx.emit('post_tool_call', tool_name='terminal', result={'output':'UNATTRIBUTED TOOL'})
ctx.emit('on_stream_delta', turn_id='foreign-turn', iteration=1, kind='text', delta='UNATTRIBUTED STREAM')
assert json.loads(status.read_text())['state'] == 'idle'
p = json.loads(preview.read_text())
assert p['activity']['text'] == 'PUBLIC' and p['edit']['filename'] == 'file'
assert 'PRIVATE' not in preview.read_text() and 'OTHER' not in preview.read_text()

ctx.emit('subagent_start', parent_session_id='one', child_session_id='child', child_goal='Review')
ctx.emit('subagent_start', parent_session_id='one', child_session_id='child', child_goal='Duplicate')
ctx.emit('subagent_start', parent_session_id='foreign', child_session_id='foreign-child', child_goal='Foreign')
ctx.emit('subagent_start', parent_session_id='one', child_session_id='../escape', child_goal='Unsafe')
ctx.emit('subagent_start', parent_session_id='one', child_session_id='one', child_goal='Self')
ctx.emit('subagent_start', parent_session_id='child', child_session_id='nested', child_goal='Nested')
ctx.emit('on_stream_delta', session_id='child', turn_id='child-turn', iteration=1, kind='text', delta='CHILD')
ctx.emit('post_llm_call', session_id='child', turn_id='child-turn', assistant_response='CHILD PUBLIC')
ctx.emit('on_stream_delta', session_id='child', turn_id='child-turn', iteration=1, kind='text', delta='LATE CHILD')
child_path = cache / 'previews' / (str(os.getpid()) + '-child.json')
assert json.loads(child_path.read_text())['activity']['text'] == 'CHILD PUBLIC'
assert json.loads(preview.read_text())['activity']['text'] == 'PUBLIC'
assert len(json.loads(status.read_text())['children']) == 2
ctx.emit('subagent_stop', child_session_id='child', child_status='completed', child_summary='CHILD DONE')
assert json.loads(status.read_text())['children'][0]['state'] == 'idle'
assert json.loads(child_path.read_text())['activity']['text'] == 'CHILD DONE'
ctx.emit('subagent_stop', child_session_id='nested', child_status='interrupted')
assert json.loads(status.read_text())['children'][1]['state'] == 'error'
old_ctx = ctx
reload_ctx = Context()
module.register(reload_ctx)
ctx = reload_ctx
ctx.emit('post_llm_call', session_id='child', assistant_response='AFTER RELOAD')
assert json.loads(status.read_text())['children'][0]['sessionID'] == 'child'
old_ctx.close()
assert json.loads(status.read_text())['children'][0]['sessionID'] == 'child'
assert json.loads(child_path.read_text())['activity']['text'] == 'AFTER RELOAD'
ctx.emit('pre_command', session_key='one', command='undo')
assert json.loads(preview.read_text())['activity'] is None

ctx.emit('on_session_reset', session_id='two')
assert json.loads(preview.read_text())['sessionID'] == 'two'
assert json.loads(status.read_text())['children'] == []
assert not child_path.exists()
ctx.emit('pre_llm_call', session_id='two')

new = Context()
module.register(new)
new.emit('on_stream_delta', session_id='two', turn_id='running-turn', iteration=1, kind='text', delta='RELOADED')
assert json.loads(status.read_text())['state'] == 'running'
ctx.emit('post_llm_call', session_id='one', assistant_response='OLD HANDLER')
new.emit('on_session_start', session_id='new', platform='cli')
assert json.loads(status.read_text())['sessionID'] == 'new'

new.close()
assert not status.exists() and not preview.exists()
`
	output, err := exec.Command(python, "-c", script, path, root).CombinedOutput()
	if err != nil {
		t.Fatalf("native observer: %v\n%s", err, output)
	}
}

func TestInstallObserverPreservesUnrelatedPlugin(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "__init__.py"), []byte("# user plugin"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := installObserver(directory); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatal("overwrote user plugin")
	}
	if _, err := os.Stat(filepath.Join(directory, "plugin.yaml")); !os.IsNotExist(err) {
		t.Fatal("partially installed over user plugin")
	}
}
