document.addEventListener('DOMContentLoaded', () => {
  const intro = document.querySelector('.intro');
  if (intro) {
    setTimeout(() => intro.classList.add('is-hidden'), 1600);
    setTimeout(() => intro.remove(), 2300);
  }

});
