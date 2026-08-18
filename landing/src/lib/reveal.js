const options = { rootMargin: '0px 0px -10% 0px', threshold: 0.08 };

export function reveal(node, { delay = 0, onreveal } = {}) {
  if (delay) node.style.setProperty('--reveal-delay', `${delay}ms`);

  const observer = new IntersectionObserver(([entry]) => {
    if (!entry.isIntersecting) return;
    node.setAttribute('data-revealed', '');
    onreveal?.();
    observer.disconnect();
  }, options);

  observer.observe(node);
  return { destroy: () => observer.disconnect() };
}
