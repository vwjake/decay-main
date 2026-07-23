document.addEventListener('DOMContentLoaded', () => {
  const intro = document.querySelector('.intro');
  if (intro) {
    setTimeout(() => intro.classList.add('is-hidden'), 1600);
    setTimeout(() => intro.remove(), 2300);
  }

  const form = document.querySelector('.signup-form');
  if (form) {
    const button = form.querySelector('.signup-submit');
    const defaultLabel = button.textContent;
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      button.textContent = 'THANKS!';
      form.reset();
      setTimeout(() => { button.textContent = defaultLabel; }, 2500);
    });
  }
});
