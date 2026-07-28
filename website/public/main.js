// DaemonHound Website — client JS

function initSlideshow() {
  const overlay = document.getElementById('slideshowOverlay');
  const img = document.getElementById('slideshowImage');
  const counter = document.getElementById('slideshowCounter');
  const dots = document.getElementById('slideshowDots');
  if (!overlay || !img || !counter || !dots) return;

  const total = 6;
  let current = 0;

  function update() {
    img.classList.add('switching');
    setTimeout(() => {
      img.src = `/images/${current + 1}.png`;
      img.classList.remove('switching');
    }, 80);
    counter.textContent = `${current + 1} / ${total}`;
    dots.querySelectorAll('.slideshow-dot').forEach((d, i) => {
      d.classList.toggle('active', i === current);
    });
  }

  function open() { overlay.classList.add('open'); overlay.setAttribute('aria-hidden', 'false'); }
  function close() { overlay.classList.remove('open'); overlay.setAttribute('aria-hidden', 'true'); }
  function prev() { current = (current - 1 + total) % total; update(); }
  function next() { current = (current + 1) % total; update(); }
  function goTo(i) { current = i; update(); }

  document.addEventListener('click', (e) => {
    if (e.target.closest('#showSlideshow')) { open(); return; }
    if (e.target.closest('#slideshowClose')) { close(); return; }
    if (e.target.closest('#slideshowPrev')) { prev(); return; }
    if (e.target.closest('#slideshowNext')) { next(); return; }
    const dot = e.target.closest('.slideshow-dot');
    if (dot) { goTo(parseInt(dot.dataset.index)); return; }
    if (e.target === overlay) { close(); }
  });

  document.addEventListener('keydown', (e) => {
    if (!overlay.classList.contains('open')) return;
    if (e.key === 'Escape') { close(); }
    if (e.key === 'ArrowLeft') { prev(); }
    if (e.key === 'ArrowRight') { next(); }
  });
}

document.addEventListener('DOMContentLoaded', () => {

  const toggle = document.querySelector('.mobile-toggle');
  const navLinks = document.querySelector('.nav-links');
  if (toggle && navLinks) {
    toggle.addEventListener('click', () => navLinks.classList.toggle('open'));
    navLinks.querySelectorAll('a').forEach(link => {
      link.addEventListener('click', () => navLinks.classList.remove('open'));
    });
  }

  // Copy buttons
  document.querySelectorAll('[data-copy]').forEach(btn => {
    btn.addEventListener('click', () => {
      navigator.clipboard.writeText(btn.getAttribute('data-copy') || '').then(() => {
        const orig = btn.innerHTML;
        btn.classList.add('copied');
        btn.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><polyline points="20 6 9 17 4 12"/></svg> Copied!`;
        setTimeout(() => { btn.classList.remove('copied'); btn.innerHTML = orig; }, 2000);
      });
    });
  });

  initSlideshow();

  // Re-init slideshow after Astro SPA navigations
  document.addEventListener('astro:after-swap', initSlideshow);
});
