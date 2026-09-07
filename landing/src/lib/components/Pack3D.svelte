<script>
  import { onMount } from 'svelte';
  import * as THREE from 'three';
  import { reveal } from '$lib/reveal.js';

  let { delay = 0, size = 320, pixels = 128, spin = true, intro = true, slide = false } = $props();
  let canvas;

  const RUST = 0xc4522a, RUST_D = 0x9e4220, GREEN = 0x2f5d40, GREEN_D = 0x244a39,
    BROWN = 0x6b3b1e, SILVER = 0xc2bfb5, ROLL = 0xb39a68, ROLL_D = 0x8c7550, CHARCOAL = 0x2b2b2f, STRAP = 0x3a2a1c;

  function mat(color) {
    return new THREE.MeshLambertMaterial({ color, flatShading: true });
  }

  function box(w, h, d, color, x, y, z) {
    const m = new THREE.Mesh(new THREE.BoxGeometry(w, h, d), mat(color));
    m.position.set(x, y, z);
    return m;
  }

  function buckle(x, y, z) {
    const g = new THREE.Group();
    g.add(box(0.22, 0.2, 0.08, SILVER, 0, 0, 0));
    g.add(box(0.12, 0.1, 0.1, STRAP, 0, 0, 0.01));
    g.position.set(x, y, z);
    return g;
  }

  function build() {
    const pack = new THREE.Group();
    const F = 0.47;

    const body = new THREE.Mesh(new THREE.BoxGeometry(1.6, 2.0, 0.9, 2, 3, 2), mat(RUST));
    const pos = body.geometry.attributes.position;
    for (let i = 0; i < pos.count; i++) {
      const y = pos.getY(i), x = pos.getX(i), z = pos.getZ(i);
      const mid = 1 - Math.min(1, Math.abs(y) / 1.0);
      pos.setX(i, x * (1 + 0.06 * mid));
      pos.setZ(i, z * (1 + 0.12 * mid));
    }
    body.geometry.computeVertexNormals();
    body.position.y = 0.05;
    pack.add(body);

    pack.add(box(1.66, 0.5, 0.96, GREEN, 0, -0.75, 0));

    const hinge = new THREE.Group();
    hinge.position.set(0, 0.75, -0.47);
    pack.add(hinge);
    const onLid = (m) => { m.position.sub(hinge.position); hinge.add(m); return m; };
    onLid(box(1.72, 0.46, 1.02, RUST_D, 0, 0.98, 0.04));
    onLid(box(1.72, 0.12, 0.2, RUST_D, 0, 0.72, 0.5));

    const rollHinge = new THREE.Group();
    const rollPivot = new THREE.Vector3(0, 1.2, -0.45);
    rollHinge.position.copy(rollPivot).sub(hinge.position);
    hinge.add(rollHinge);
    const onRoll = (m) => { m.position.sub(rollPivot); rollHinge.add(m); return m; };

    const roll = new THREE.Mesh(new THREE.CylinderGeometry(0.31, 0.31, 1.95, 8), mat(ROLL));
    roll.rotation.z = Math.PI / 2;
    roll.position.set(0, 1.5, 0.06);
    onRoll(roll);
    for (const side of [-1, 1]) {
      const cap = new THREE.Mesh(new THREE.CylinderGeometry(0.2, 0.2, 0.04, 8), mat(ROLL_D));
      cap.rotation.z = Math.PI / 2;
      cap.position.set(side * 0.975, 1.5, 0.06);
      onRoll(cap);
    }

    for (const side of [-1, 1]) {
      const x = side * 0.46;
      pack.add(box(0.18, 0.86, 0.06, BROWN, x, 0.28, F + 0.06));
      onLid(box(0.18, 0.46, 0.06, BROWN, x, 0.94, F + 0.06));
      onRoll(box(0.18, 0.06, 0.74, BROWN, x, 1.83, 0.06));
      onRoll(box(0.18, 0.5, 0.06, BROWN, x, 1.55, 0.4));
      onRoll(box(0.18, 0.5, 0.06, BROWN, x, 1.55, -0.28));
      pack.add(buckle(x, 0.02, F + 0.1));
    }
    pack.userData.hinge = hinge;

    pack.add(box(1.68, 0.14, 0.06, BROWN, 0, 0.3, F + 0.05));
    pack.add(buckle(0.62, 0.3, F + 0.11));

    pack.add(box(1.3, 0.62, 0.42, GREEN, 0, -0.44, F + 0.2));
    pack.add(box(1.34, 0.22, 0.46, GREEN_D, 0, -0.14, F + 0.22));
    for (const side of [-1, 1]) {
      const x = side * 0.36;
      pack.add(box(0.16, 0.6, 0.06, BROWN, x, -0.36, F + 0.44));
      pack.add(buckle(x, -0.58, F + 0.48));
    }

    for (const side of [-1, 1]) {
      pack.add(box(0.34, 0.8, 0.62, GREEN, side * 0.94, -0.15, 0.02));
      pack.add(box(0.36, 0.2, 0.64, GREEN_D, side * 0.94, 0.33, 0.02));
    }

    for (const side of [-1, 1]) {
      const strap = box(0.24, 1.9, 0.12, STRAP, side * 0.42, 0, -0.55);
      strap.rotation.x = -0.22;
      strap.rotation.z = side * -0.1;
      pack.add(strap);
      pack.add(box(0.26, 0.9, 0.16, CHARCOAL, side * 0.42, 0.55, -0.52));
    }

    const handle = new THREE.Mesh(new THREE.TorusGeometry(0.2, 0.05, 4, 8), mat(STRAP));
    handle.position.set(0, 1.28, -0.4);
    pack.add(handle);

    return pack;
  }

  let playing = $state(false);
  let frame = 0;
  let begun = 0;
  let tickRef = () => {};
  const OPEN = -1.25;
  const back = (t) => 1 + 2.4 * Math.pow(t - 1, 3) + 1.4 * Math.pow(t - 1, 2);
  const clamp = (t) => Math.min(1, Math.max(0, t));

  onMount(() => {
    const renderer = new THREE.WebGLRenderer({ canvas, antialias: false, alpha: true, preserveDrawingBuffer: true });
    renderer.setPixelRatio(1);
    renderer.setSize(pixels, pixels, false);

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(30, 1, 0.1, 50);
    camera.position.set(0, 1.6, 7.4);
    camera.lookAt(0, 0.3, 0);

    scene.add(new THREE.AmbientLight(0xfff1dc, 0.7));
    const key = new THREE.DirectionalLight(0xfff4e0, 2.6);
    key.position.set(-1.8, 3.2, 4);
    const fill = new THREE.DirectionalLight(0xffd9b0, 0.5);
    fill.position.set(3, 0.5, 2);
    scene.add(fill);
    scene.add(key);
    const rim = new THREE.DirectionalLight(0x9dbf88, 0.6);
    rim.position.set(3, 1, -3);
    scene.add(rim);

    const pack = build();
    pack.position.y = -0.3;
    const root = new THREE.Group();
    root.position.y = 0.3;
    root.rotation.y = -0.5;
    root.add(pack);
    scene.add(root);

    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const still = !spin || reduced;
    const hinge = pack.userData.hinge;
    const arriving = intro && !reduced;
    if (arriving) hinge.rotation.x = OPEN;

    const tick = () => {
      if (!playing) return;
      const now = performance.now();
      let settling = false;
      if (arriving && begun) {
        const t = (now - begun) / 1000;
        const lid = 1 - Math.pow(1 - clamp((t - 0.3) / 0.55), 3);
        const thump = t > 0.85 && t < 1.0 ? 0.05 * Math.sin(Math.PI * (t - 0.85) / 0.15) : 0;
        hinge.rotation.x = OPEN * (1 - lid) + thump;
        settling = t < 1.05;
      }
      if (!still && (!arriving || begun)) {
        root.rotation.y += 0.008;
        root.position.y = 0.3 + Math.sin(now / 900) * 0.05;
      }
      renderer.render(scene, camera);
      if (still && !settling && !(arriving && !begun)) return;
      frame = requestAnimationFrame(tick);
    };
    tickRef = tick;
    playing = true;
    tick();
    if (!arriving) begun = performance.now();

    if (!window.zainoPack || window.zainoPack.pixels < pixels) {
      window.zainoPack = {
        pixels,
        pose(y) { root.rotation.y = y; root.position.y = 0.3; root.scale.setScalar(1); hinge.rotation.x = 0; renderer.render(scene, camera); return canvas.toDataURL('image/png'); }
      };
    }

    return () => {
      playing = false;
      cancelAnimationFrame(frame);
      renderer.dispose();
    };
  });
</script>

<div class="pack3d-wrap" class:slide data-reveal use:reveal={{ delay, onreveal: () => { if (!begun) { begun = performance.now(); if (playing) { cancelAnimationFrame(frame); frame = requestAnimationFrame(tickRef); } } } }}>
  <canvas bind:this={canvas} class="pack3d" width={pixels} height={pixels} style:width="{size}px" style:height="{size}px" aria-label="A low-poly backpack, turning"></canvas>
</div>

<style>
  .pack3d-wrap { display: grid; place-items: center; }
  :root[data-js] .pack3d-wrap:not([data-revealed]) { translate: 0 0; }
  :root[data-js] .pack3d-wrap.slide:not([data-revealed]) { translate: 28px 0; }
  .pack3d {
    image-rendering: pixelated;
    image-rendering: crisp-edges;
    max-width: 100%;
    height: auto;
    aspect-ratio: 1;
    filter: drop-shadow(8px 10px 0 var(--shadow));
  }
</style>
