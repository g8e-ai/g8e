const menuButton = document.querySelector('.menu-button');
const pageNav = document.querySelector('.page-nav');
const progress = document.querySelector('.progress span');

menuButton?.addEventListener('click', () => {
  const open = menuButton.getAttribute('aria-expanded') === 'true';
  menuButton.setAttribute('aria-expanded', String(!open));
  pageNav.classList.toggle('open', !open);
});

pageNav?.addEventListener('click', event => {
  if (event.target instanceof HTMLAnchorElement) {
    menuButton?.setAttribute('aria-expanded', 'false');
    pageNav.classList.remove('open');
  }
});

const headings = [...document.querySelectorAll('.readme h2[id]')];
const links = new Map([...document.querySelectorAll('.page-nav a[href^="#"]')].map(link => [link.hash.slice(1), link]));
const observer = new IntersectionObserver(entries => {
  const visible = entries.filter(entry => entry.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0];
  if (!visible) return;
  links.forEach(link => link.removeAttribute('aria-current'));
  links.get(visible.target.id)?.setAttribute('aria-current', 'true');
}, { rootMargin: '-15% 0px -75% 0px' });
headings.forEach(heading => observer.observe(heading));

const updateProgress = () => {
  const scrollable = document.documentElement.scrollHeight - window.innerHeight;
  progress.style.transform = `scaleX(${scrollable > 0 ? window.scrollY / scrollable : 0})`;
};
window.addEventListener('scroll', updateProgress, { passive: true });
updateProgress();

const mermaidBlocks = document.querySelectorAll('.mermaid');
if (mermaidBlocks.length) {
  import('https://cdn.jsdelivr.net/npm/mermaid@11.12.0/dist/mermaid.esm.min.mjs').then(({ default: mermaid }) => {
    mermaid.initialize({ startOnLoad: false, theme: 'dark', securityLevel: 'strict', fontFamily: 'ui-monospace, monospace' });
    return mermaid.run({ nodes: mermaidBlocks });
  }).catch(() => {
    mermaidBlocks.forEach(block => block.classList.add('mermaid-fallback'));
  });
}
