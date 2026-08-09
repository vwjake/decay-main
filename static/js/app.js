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

  // Live character counts for capped fields (the account blurb). The
  // textarea's own maxlength does the enforcing; this just says how much
  // room is left. Without JS the count is simply the value on page load.
  document.querySelectorAll('textarea[data-counter]').forEach((field) => {
    const out = document.getElementById(field.dataset.counter);
    if (!out) return;
    const update = () => { out.textContent = String(field.value.length); };
    field.addEventListener('input', update);
    update();
  });

  pollOrderStatus();
  setupGallery();
  setupShareButtons();
});

// setupShareButtons wires up "Share" buttons on event and group pages: the
// native share sheet where the browser has one (mostly mobile), falling
// back to copying the link since desktop browsers mostly don't.
function setupShareButtons() {
  document.querySelectorAll('[data-share]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const url = new URL(btn.dataset.share, window.location.origin).href;
      if (navigator.share) {
        try {
          await navigator.share({ title: document.title, url });
        } catch (e) {
          // AbortError just means the person closed the share sheet.
        }
        return;
      }
      if (navigator.clipboard) {
        try {
          await navigator.clipboard.writeText(url);
          const original = btn.textContent;
          btn.textContent = 'Link copied!';
          window.setTimeout(() => { btn.textContent = original; }, 2000);
        } catch (e) {
          window.prompt('Copy this link:', url);
        }
      } else {
        window.prompt('Copy this link:', url);
      }
    });
  });
}

// pollOrderStatus fills in the order confirmation page when it was reached
// before Stripe's webhook arrived. The buyer is redirected back the instant
// they pay, so a pending order is normal for a second or two rather than a
// fault. Confirmation reloads the page instead of drawing the paid state
// here, so there's one piece of code that knows what a paid order looks
// like. Without JS the page still shows the order and says it's pending —
// a refresh does the same job by hand.
function pollOrderStatus() {
  const pending = document.querySelector('[data-order-poll]');
  if (!pending) return;

  const token = pending.dataset.orderPoll;
  const retryMs = 3000;
  let attempts = 0;

  const tick = () => {
    // Roughly two minutes. A webhook that hasn't landed by then isn't
    // going to be fixed by asking again, so say so rather than spin.
    if (attempts >= 40) {
      pending.textContent = 'Still confirming — refresh in a minute, or email us and quote your order.';
      return;
    }
    attempts += 1;

    fetch('/api/order-status?token=' + encodeURIComponent(token))
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data && data.status === 'paid') {
          window.location.reload();
          return;
        }
        window.setTimeout(tick, retryMs);
      })
      .catch(() => window.setTimeout(tick, retryMs));
  };

  window.setTimeout(tick, 2000);
}

// setupGallery turns the /photos grid into a clickable lightbox: a photo
// opens full-size over a dimmed page, with prev/next, keyboard arrows, and
// Esc to close. Progressive enhancement — without JS each photo is still a
// plain link to its full-size file.
function setupGallery() {
  const gallery = document.querySelector('.gallery');
  if (!gallery) return;
  const links = Array.from(gallery.querySelectorAll('.gallery-item a'));
  if (links.length === 0) return;

  const items = links.map((a) => {
    const caption = a.closest('.gallery-item').querySelector('.gallery-caption');
    return { href: a.getAttribute('href'), caption: caption ? caption.textContent : '' };
  });

  const box = document.createElement('div');
  box.className = 'lightbox';
  box.setAttribute('role', 'dialog');
  box.setAttribute('aria-modal', 'true');
  box.hidden = true;
  box.innerHTML =
    '<button class="lightbox-close" type="button" aria-label="Close">×</button>' +
    '<button class="lightbox-nav lightbox-prev" type="button" aria-label="Previous photo">‹</button>' +
    '<figure class="lightbox-figure">' +
    '<img class="lightbox-img" alt=""/>' +
    '<figcaption class="lightbox-caption mono"></figcaption>' +
    '</figure>' +
    '<button class="lightbox-nav lightbox-next" type="button" aria-label="Next photo">›</button>';
  document.body.appendChild(box);

  const img = box.querySelector('.lightbox-img');
  const caption = box.querySelector('.lightbox-caption');
  const nav = box.querySelectorAll('.lightbox-nav');
  // A single photo needs no next/previous controls.
  if (items.length < 2) nav.forEach((btn) => { btn.hidden = true; });

  let current = 0;
  const show = (i) => {
    current = (i + items.length) % items.length;
    img.src = items[current].href;
    caption.textContent = items[current].caption;
    caption.hidden = !items[current].caption;
  };
  const open = (i) => {
    show(i);
    box.hidden = false;
    document.body.classList.add('lightbox-open');
  };
  const close = () => {
    box.hidden = true;
    img.removeAttribute('src');
    document.body.classList.remove('lightbox-open');
  };

  links.forEach((a, i) => a.addEventListener('click', (e) => { e.preventDefault(); open(i); }));
  box.querySelector('.lightbox-close').addEventListener('click', close);
  box.querySelector('.lightbox-prev').addEventListener('click', () => show(current - 1));
  box.querySelector('.lightbox-next').addEventListener('click', () => show(current + 1));
  // A click on the backdrop (but not the image or a control) closes it.
  box.addEventListener('click', (e) => { if (e.target === box) close(); });
  document.addEventListener('keydown', (e) => {
    if (box.hidden) return;
    if (e.key === 'Escape') close();
    else if (e.key === 'ArrowLeft') show(current - 1);
    else if (e.key === 'ArrowRight') show(current + 1);
  });

  // /media shows only the first photo, with a button to browse the rest —
  // the other photos are already in the DOM (just hidden), so opening the
  // lightbox at index 0 gives access to all of them via prev/next.
  const galleryOpen = document.querySelector('[data-gallery-open]');
  if (galleryOpen) galleryOpen.addEventListener('click', () => open(0));
}

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
