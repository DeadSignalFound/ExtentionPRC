(() => {
  const host = location.hostname;
  const isMusic = host === "music.youtube.com";
  const isYT = !isMusic && (host === "www.youtube.com" || host === "youtube.com" || host === "m.youtube.com");
  if (!isMusic && !isYT) return;

  let lastKey = "";

  const txt = (el) => (el && el.textContent || "").replace(/\s+/g, " ").trim();

  function pickVideo() {
    return document.querySelector("video.html5-main-video") || document.querySelector("video");
  }

  function scrapeYTMusic() {
    const v = pickVideo();
    if (!v) return null;
    const bar = document.querySelector("ytmusic-player-bar");
    if (!bar) return null;

    const title = txt(bar.querySelector(".title.ytmusic-player-bar")) ||
                  txt(document.querySelector(".content-info-wrapper .title"));
    const bylineEl = bar.querySelector(".byline.ytmusic-player-bar") ||
                     bar.querySelector(".subtitle.ytmusic-player-bar");
    const byline = txt(bylineEl);
    const parts = byline.split("•").map(p => p.trim()).filter(Boolean);
    const artist = parts[0] || "";
    const album = parts[1] || "";

    let thumb = "";
    const img = bar.querySelector("img.image.ytmusic-player-bar") || bar.querySelector("img");
    if (img && img.src) {
      thumb = img.src.replace(/=w\d+-h\d+[^=]*$/, "=w300-h300-l90-rj");
    }

    if (!title) return null;
    return {
      app: "YouTube Music",
      title, artist, album, thumb,
      paused: v.paused || v.ended,
      currentTime: v.currentTime || 0,
      duration: Number.isFinite(v.duration) ? v.duration : 0,
      url: location.href,
    };
  }

  function scrapeYT() {
    if (!location.pathname.startsWith("/watch")) return null;
    const v = pickVideo();
    if (!v) return null;

    const title = txt(document.querySelector("h1.ytd-watch-metadata yt-formatted-string")) ||
                  txt(document.querySelector("h1.title")) ||
                  document.title.replace(/ - YouTube$/, "").trim();
    const channel = txt(document.querySelector("ytd-channel-name#channel-name a")) ||
                    txt(document.querySelector("#owner #channel-name a")) ||
                    txt(document.querySelector("#channel-name a")) ||
                    txt(document.querySelector("#owner-name a")) || "";

    const vid = new URL(location.href).searchParams.get("v");
    const thumb = vid ? `https://i.ytimg.com/vi/${vid}/mqdefault.jpg` : "";

    if (!title) return null;
    return {
      app: "YouTube",
      title, artist: channel, album: "", thumb,
      paused: v.paused || v.ended,
      currentTime: v.currentTime || 0,
      duration: Number.isFinite(v.duration) ? v.duration : 0,
      url: location.href,
    };
  }

  function tick() {
    const d = isMusic ? scrapeYTMusic() : scrapeYT();
    if (!d) return;
    const bucket = Math.floor((d.currentTime || 0) / 5);
    const k = [d.title, d.artist, d.paused ? 1 : 0, bucket, d.url].join("|");
    if (k === lastKey) return;
    lastKey = k;
    try { browser.runtime.sendMessage({ kind: "media", data: d }); } catch {}
  }

  function bindVideo() {
    const v = pickVideo();
    if (!v || v.__rpcBound) return;
    v.__rpcBound = true;
    ["play", "pause", "seeked", "ratechange", "ended", "loadedmetadata"].forEach(ev =>
      v.addEventListener(ev, tick)
    );
  }

  setInterval(() => { bindVideo(); tick(); }, 2000);
  tick();

  let lastHref = location.href;
  new MutationObserver(() => {
    if (location.href !== lastHref) {
      lastHref = location.href;
      lastKey = "";
      setTimeout(tick, 500);
      setTimeout(tick, 1500);
    }
  }).observe(document, { subtree: true, childList: true });
})();
