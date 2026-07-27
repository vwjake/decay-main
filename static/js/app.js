document.addEventListener('DOMContentLoaded', () => {
  const intro = document.querySelector('.intro');
  if (intro) {
    setTimeout(() => intro.classList.add('is-hidden'), 1600);
    setTimeout(() => intro.remove(), 2300);
  }

  // Markdown toolbars: each .md-editor pairs a button row with a textarea,
  // and its buttons wrap or prefix the current selection with Markdown.
  document.querySelectorAll('.md-editor').forEach((editor) => {
    const textarea = editor.querySelector('textarea');
    if (!textarea) return;

    editor.querySelectorAll('[data-md]').forEach((btn) => {
      btn.addEventListener('click', (e) => {
        e.preventDefault();
        if (btn.dataset.md === 'embed') embedLink(textarea);
        else applyMarkdown(textarea, btn.dataset.md);
      });
    });

    // Ctrl/Cmd+B and +I match what people expect from any editor.
    textarea.addEventListener('keydown', (e) => {
      if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
      const key = e.key.toLowerCase();
      if (key === 'b') { e.preventDefault(); applyMarkdown(textarea, 'bold'); }
      else if (key === 'i') { e.preventDefault(); applyMarkdown(textarea, 'italic'); }
    });
  });

  // "Insert" buttons (uploaded images, resolved embeds) drop a ready-made
  // Markdown snippet into the body at the cursor. There's one body editor
  // per page, so they all target it.
  const bodyEditor = document.querySelector('.md-editor textarea');
  document.querySelectorAll('[data-insert-md]').forEach((btn) => {
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      if (bodyEditor) insertAtCursor(bodyEditor, btn.dataset.insertMd);
    });
  });
});

// embedLink asks for a YouTube or Bandcamp URL, has the server resolve it
// to an embeddable link (a Bandcamp page needs a lookup), and inserts that
// on its own line — the Markdown renderer turns such a line into a player.
async function embedLink(textarea) {
  const url = window.prompt('Paste a YouTube or Bandcamp link to embed:');
  if (!url) return;
  try {
    const res = await fetch('/admin/posts/embed', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: 'url=' + encodeURIComponent(url.trim()),
    });
    const data = await res.json();
    if (data.error) { window.alert(data.error); return; }
    insertAtCursor(textarea, data.insert);
  } catch (e) {
    window.alert('Could not reach the server to resolve that link.');
  }
}

// insertAtCursor drops text into the textarea at the caret (or over the
// selection), on its own line, and leaves the caret after it.
function insertAtCursor(textarea, text) {
  const start = textarea.selectionStart;
  const value = textarea.value;
  // Give block-level snippets (images, embeds) their own paragraph.
  const before = start > 0 && value[start - 1] !== '\n' ? '\n\n' : '';
  const snippet = before + text;
  textarea.setRangeText(snippet, start, textarea.selectionEnd, 'end');
  textarea.focus();
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
}

// applyMarkdown mutates the textarea's current selection in place, keeping
// the browser's native undo history via setRangeText.
function applyMarkdown(textarea, action) {
  const value = textarea.value;
  const start = textarea.selectionStart;
  const end = textarea.selectionEnd;
  const selected = value.slice(start, end);

  // wrap surrounds the selection with before/after; with nothing selected
  // it drops in a placeholder and leaves it selected for typing over.
  const wrap = (before, after, placeholder) => {
    const inner = selected || placeholder;
    textarea.setRangeText(before + inner + after, start, end, 'end');
    if (!selected) {
      textarea.selectionStart = start + before.length;
      textarea.selectionEnd = start + before.length + placeholder.length;
    }
  };

  // linePrefix adds prefix to the start of every line the selection touches,
  // so a list or quote button works on a whole block at once.
  const linePrefix = (prefix) => {
    const lineStart = value.lastIndexOf('\n', start - 1) + 1;
    let lineEnd = value.indexOf('\n', end);
    if (lineEnd === -1) lineEnd = value.length;
    const block = value.slice(lineStart, lineEnd);
    const prefixed = block.split('\n').map((line) => prefix + line).join('\n');
    textarea.setRangeText(prefixed, lineStart, lineEnd, 'end');
  };

  switch (action) {
    case 'bold': wrap('**', '**', 'bold text'); break;
    case 'italic': wrap('*', '*', 'italic text'); break;
    case 'heading': linePrefix('## '); break;
    case 'ul': linePrefix('- '); break;
    case 'quote': linePrefix('> '); break;
    case 'code':
      if (selected.includes('\n')) wrap('```\n', '\n```', 'code');
      else wrap('`', '`', 'code');
      break;
    case 'link': {
      const text = selected || 'link text';
      const prefix = '[' + text + '](';
      textarea.setRangeText(prefix + 'https://)', start, end, 'end');
      // Leave the URL selected so it can be typed or pasted straight over.
      textarea.selectionStart = start + prefix.length;
      textarea.selectionEnd = start + prefix.length + 'https://'.length;
      break;
    }
  }

  textarea.focus();
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
}
