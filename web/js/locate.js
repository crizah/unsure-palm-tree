(() => {
  'use strict';

  // Basemap: one place to swap providers (e.g. self-hosted tiles). OSM's public
  // server is for development and light use only; see tile usage policy.
  const TILE_URL = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
  const TILE_ATTR = '© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener noreferrer">OpenStreetMap</a> contributors';
  const TYPES = ['Government', 'NGO', 'Religious'];
  const DEFAULT_VIEW = { center: [20.59, 78.96], zoom: 5 };
  const RADIUS_KM = 5;

  const state = { city: '', types: new Set(), q: '', me: null, places: [], selected: null };
  const markers = new Map(); // place id -> L.Marker
  const cards = new Map();   // place id -> <li>

  const $ = (id) => document.getElementById(id);
  const panel = $('panel'), listEl = $('list'), statusEl = $('status'), searchEl = $('search');

  const map = L.map('map', { zoomControl: false, attributionControl: true }).setView(DEFAULT_VIEW.center, DEFAULT_VIEW.zoom);
  map.attributionControl.setPrefix(false);
  L.control.zoom({ position: 'topright' }).addTo(map);
  L.tileLayer(TILE_URL, { attribution: TILE_ATTR, maxZoom: 19, referrerPolicy: 'origin' }).addTo(map);
  const pinLayer = L.layerGroup().addTo(map);
  const meLayer = L.layerGroup().addTo(map);

  // Small DOM helper. Text is always set via textContent, never innerHTML.
  function el(tag, attrs, ...kids) {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (k === 'class') n.className = v; else if (v != null) n.setAttribute(k, v);
    }
    for (const k of kids) if (k != null) n.append(k);
    return n;
  }

  // ---------- data ----------
  let reqSeq = 0;
  async function load({ fit = true } = {}) {
    const seq = ++reqSeq;
    const p = new URLSearchParams();
    if (state.me) { p.set('lat', state.me.lat); p.set('lng', state.me.lng); p.set('radius', state.me.radius); }
    if (state.city) p.set('city', state.city);
    if (state.types.size) p.set('type', [...state.types].join(','));
    if (state.q) p.set('q', state.q);
    p.set('limit', '100');
    setStatus('Loading…');
    try {
      const res = await fetch('/api/places?' + p);
      if (!res.ok) throw new Error(res.status === 429 ? 'Too many requests — try again in a moment.' : 'Could not load places.');
      const data = await res.json();
      if (seq !== reqSeq) return; // a newer request superseded this one
      state.places = data.places;
      render(fit);
    } catch (e) {
      if (seq === reqSeq) setStatus(e.message || 'Could not load places.', true);
    }
  }

  async function loadCities() {
    try {
      const res = await fetch('/api/cities');
      const { cities } = await res.json();
      const wrap = $('city-chips');
      wrap.replaceChildren(chip('All cities', '', true, () => setCity('')));
      for (const c of cities) wrap.append(chip(c.city, c.city, false, () => setCity(c.city)));
    } catch { /* city chips are optional; location and search still work */ }
  }

  // ---------- rendering ----------
  function chip(label, value, pressed, onClick, swatch) {
    const b = el('button', { class: 'chip', type: 'button', 'aria-pressed': String(pressed), 'data-v': value }, swatch ? el('span', { class: 'sw ' + swatch }) : null, label);
    b.addEventListener('click', onClick);
    return b;
  }

  function renderTypeChips() {
    const wrap = $('type-chips');
    wrap.replaceChildren();
    for (const t of TYPES) wrap.append(chip(t, t, state.types.has(t), () => toggleType(t), t));
  }

  function syncChips() {
    for (const b of $('city-chips').children) b.setAttribute('aria-pressed', String(b.dataset.v === state.city));
    for (const b of $('type-chips').children) b.setAttribute('aria-pressed', String(state.types.has(b.dataset.v)));
  }

  function setStatus(msg, isError) {
    statusEl.textContent = msg;
    statusEl.classList.toggle('error', !!isError);
  }

  function pinIcon(type, selected) {
    return L.divIcon({ className: '', html: `<div class="pin ${type}${selected ? ' sel' : ''}"></div>`, iconSize: [18, 18], iconAnchor: [9, 9] });
  }

  function render(fit) {
    pinLayer.clearLayers(); markers.clear(); cards.clear(); listEl.replaceChildren();
    const mapped = [];
    for (const p of state.places) {
      const li = card(p);
      cards.set(p.id, li); listEl.append(li);
      if (p.lat != null) {
        const m = L.marker([p.lat, p.lng], { icon: pinIcon(p.type, false), title: p.name, keyboard: true });
        m.on('click', () => select(p.id, { fromMap: true }));
        m.addTo(pinLayer); markers.set(p.id, m); mapped.push(p);
      }
    }
    const n = state.places.length;
    if (n === 0) {
      setStatus(state.me ? `No places within ${state.me.radius} km. Try another city or clear filters.` : 'No places match.');
    } else {
      setStatus(state.me ? `${n} place${n === 1 ? '' : 's'} within ${state.me.radius} km` : `${n} place${n === 1 ? '' : 's'}`);
    }
    state.selected = null;
    if (fit) fitView(mapped);
  }

  function fitView(mapped) {
    const pts = mapped.map((p) => [p.lat, p.lng]);
    if (state.me) pts.push([state.me.lat, state.me.lng]);
    if (pts.length === 0) return;
    if (pts.length === 1) map.setView(pts[0], 15);
    else map.fitBounds(pts, { padding: [40, 40], maxZoom: 15, paddingBottomRight: isMobile() ? [40, 240] : [40, 40] });
  }

  function card(p) {
    const meta = el('div', { class: 'meta' });
    if (p.distance_km != null) meta.append(el('span', { class: 'dist' }, fmtDist(p.distance_km)));
    meta.append(el('span', null, p.address), el('span', null, 'Time: ' + p.timings), el('span', null, 'Cost: ' + p.cost));
    const li = el('li', { class: 'card', 'data-id': p.id, tabindex: '0' },
      el('div', { class: 'card-head' }, el('h2', null, p.name), el('span', { class: 'badge' }, p.type)),
      meta);
    if (p.lat != null) {
      const a = el('a', {
        href: `https://www.google.com/maps/dir/?api=1&destination=${p.lat},${p.lng}`,
        target: '_blank', rel: 'noopener noreferrer',
      }, 'Directions');
      a.addEventListener('click', (e) => e.stopPropagation());
      li.append(el('div', { class: 'card-actions' }, a));
      if (p.geo_precision === 'area') li.append(el('div', { class: 'nopin' }, 'Approximate location (area level)'));
    } else {
      li.append(el('div', { class: 'nopin' }, 'Multiple locations — not shown on map'));
    }
    const act = () => select(p.id);
    li.addEventListener('click', act);
    li.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); act(); } });
    return li;
  }

  function fmtDist(km) { return km < 1 ? `${Math.round(km * 1000)} m away` : `${km.toFixed(1)} km away`; }

  function select(id, { fromMap = false } = {}) {
    const prev = state.selected;
    if (prev != null) {
      cards.get(prev)?.classList.remove('selected');
      const pm = markers.get(prev), pp = state.places.find((x) => x.id === prev);
      if (pm && pp) pm.setIcon(pinIcon(pp.type, false));
    }
    state.selected = id;
    const li = cards.get(id), p = state.places.find((x) => x.id === id);
    li?.classList.add('selected');
    const m = markers.get(id);
    if (m && p) {
      m.setIcon(pinIcon(p.type, true));
      if (!fromMap) map.flyTo(m.getLatLng(), Math.max(map.getZoom(), 16), { duration: 0.6 });
      m.bindPopup(popupNode(p), { closeButton: false, offset: [0, -6] }).openPopup();
    }
    if (fromMap) li?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    if (isMobile() && !fromMap) setSheet('peek'); // reveal the map on mobile
    else if (isMobile() && fromMap && panel.dataset.state === 'full') setSheet('half');
  }

  function popupNode(p) {
    return el('div', null, el('strong', null, p.name), el('div', null, p.address), el('div', null, p.timings + ' · ' + p.cost));
  }

  // ---------- actions ----------
  function setCity(city) {
    state.city = city; state.me = null; meLayer.clearLayers();
    syncChips(); load();
  }
  function toggleType(t) {
    state.types.has(t) ? state.types.delete(t) : state.types.add(t);
    syncChips(); load({ fit: true });
  }

  let searchTimer;
  searchEl.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => { state.q = searchEl.value.trim(); load(); }, 250);
  });
  $('search-form').addEventListener('submit', (e) => { e.preventDefault(); clearTimeout(searchTimer); state.q = searchEl.value.trim(); load(); });

  const locateBtn = $('locate');
  locateBtn.addEventListener('click', () => {
    if (!('geolocation' in navigator)) { setStatus('Location is not supported on this device.', true); return; }
    locateBtn.disabled = true; setStatus('Finding your location…');
    navigator.geolocation.getCurrentPosition((pos) => {
      locateBtn.disabled = false;
      state.me = { lat: pos.coords.latitude, lng: pos.coords.longitude, radius: RADIUS_KM };
      state.city = ''; syncChips();
      meLayer.clearLayers();
      L.marker([state.me.lat, state.me.lng], { icon: L.divIcon({ className: '', html: '<div class="me"></div>', iconSize: [16, 16], iconAnchor: [8, 8] }), interactive: false, keyboard: false }).addTo(meLayer);
      L.circle([state.me.lat, state.me.lng], { radius: state.me.radius * 1000, color: '#000', weight: 1, dashArray: '4 4', fillOpacity: 0.03, interactive: false }).addTo(meLayer);
      load();
    }, (err) => {
      locateBtn.disabled = false;
      setStatus(err.code === 1 ? 'Location permission denied. Pick a city or search instead.' : 'Could not get your location. Pick a city or search instead.', true);
    }, { enableHighAccuracy: false, timeout: 10000, maximumAge: 60000 });
  });

  // ---------- mobile bottom sheet ----------
  const mq = window.matchMedia('(max-width: 720px)');
  function isMobile() { return mq.matches; }
  const STATES = ['peek', 'half', 'full'];
  function setSheet(s) { panel.dataset.state = s; panel.style.transform = ''; }

  const handle = $('handle');
  handle.addEventListener('click', () => {
    if (handle.dataset.dragged) { delete handle.dataset.dragged; return; }
    setSheet(STATES[(STATES.indexOf(panel.dataset.state) + 1) % 3]);
  });
  handle.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handle.click(); } });

  let drag = null;
  handle.addEventListener('pointerdown', (e) => {
    drag = { y: e.clientY, start: panel.getBoundingClientRect().top, h: panel.offsetHeight, moved: false };
    handle.setPointerCapture(e.pointerId); panel.classList.add('dragging');
  });
  handle.addEventListener('pointermove', (e) => {
    if (!drag) return;
    const dy = e.clientY - drag.y;
    if (Math.abs(dy) > 4) drag.moved = true;
    const top = Math.min(Math.max(drag.start + dy, window.innerHeight - drag.h), window.innerHeight - 100);
    panel.style.transform = `translateY(${top - (window.innerHeight - drag.h)}px)`;
  });
  const endDrag = () => {
    if (!drag) return;
    panel.classList.remove('dragging');
    if (drag.moved) {
      handle.dataset.dragged = '1';
      const top = panel.getBoundingClientRect().top, vh = window.innerHeight;
      const targets = { full: vh - drag.h, half: vh * 0.52 + (drag.h - vh * 0.88) * 0, peek: vh - 214 };
      let best = 'peek', bd = Infinity;
      for (const [k, v] of Object.entries(targets)) { const d = Math.abs(top - v); if (d < bd) { bd = d; best = k; } }
      setSheet(best);
    }
    drag = null;
  };
  handle.addEventListener('pointerup', endDrag);
  handle.addEventListener('pointercancel', endDrag);

  mq.addEventListener('change', () => { panel.style.transform = ''; map.invalidateSize(); });

  // ---------- boot ----------
  renderTypeChips();
  loadCities();
  load();
})();
