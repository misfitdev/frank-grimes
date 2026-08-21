// Progressive enhancement. The page is complete without this file.
(function () {
  "use strict";

  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  document.documentElement.classList.add("js");

  // Lift the install commands off the page. Every code block here is a command
  // someone is meant to run, so each one gets a copy control.
  function addCopyControls() {
    if (!navigator.clipboard) return;

    document.querySelectorAll(".code-block").forEach(function (block) {
      var button = document.createElement("button");
      button.type = "button";
      button.className = "copy-button";
      button.textContent = "Copy";
      button.setAttribute("aria-label", "Copy command to clipboard");

      var status = document.createElement("span");
      status.className = "visually-hidden";
      status.setAttribute("role", "status");

      button.addEventListener("click", function () {
        navigator.clipboard.writeText(block.innerText.trim()).then(
          function () {
            button.textContent = "Copied";
            button.classList.add("is-copied");
            status.textContent = "Command copied to clipboard";
            window.setTimeout(function () {
              button.textContent = "Copy";
              button.classList.remove("is-copied");
              status.textContent = "";
            }, 2000);
          },
          function () {
            button.textContent = "Press Cmd-C";
            window.setTimeout(function () {
              button.textContent = "Copy";
            }, 2000);
          }
        );
      });

      block.appendChild(button);
      block.appendChild(status);
    });
  }

  // Mark the section being read, the way a finger holds a place in a binder.
  function trackReadingPosition() {
    var links = new Map();
    document.querySelectorAll(".top-nav a[href^='#']").forEach(function (link) {
      var section = document.querySelector(link.getAttribute("href"));
      if (section) links.set(section, link);
    });
    if (!links.size) return;

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          links.forEach(function (link) {
            link.classList.remove("is-current");
          });
          links.get(entry.target).classList.add("is-current");
        });
      },
      { rootMargin: "-20% 0px -70% 0px" }
    );

    links.forEach(function (_link, section) {
      observer.observe(section);
    });
  }

  // Finishing the report earns the stamp. It only lands once.
  function stampOnFinish() {
    var target = document.querySelector("[data-stamp-on-read]");
    if (!target) return;

    if (reduced) {
      target.style.opacity = "0.82";
      inkBleed(target, false);
      return;
    }

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          target.classList.add("is-stamped");
          inkBleed(target, true);
          observer.disconnect();
        });
      },
      { threshold: 0.6 }
    );

    observer.observe(target);
  }

  // A sheet of paper under a light source. The grain lifts where the light
  // falls, which is the whole reason the ground is stock and not a fill.
  function rakeLight() {
    if (reduced) return;
    if (!window.matchMedia("(hover: hover) and (pointer: fine)").matches) return;

    var layer = document.createElement("div");
    layer.className = "paper-light";
    layer.setAttribute("aria-hidden", "true");
    layer.innerHTML = '<span class="glow"></span><span class="tooth"></span>';
    document.body.appendChild(layer);

    var x = 0;
    var y = 0;
    var queued = false;

    function paint() {
      queued = false;
      layer.style.transform = "translate3d(" + x + "px," + y + "px,0)";
    }

    window.addEventListener(
      "pointermove",
      function (event) {
        x = event.clientX;
        y = event.clientY;
        layer.classList.add("is-lit");
        if (queued) return;
        queued = true;
        window.requestAnimationFrame(paint);
      },
      { passive: true }
    );

    document.addEventListener("pointerleave", function () {
      layer.classList.remove("is-lit");
    });
  }

  // Wet ink spreads before it sets, and it never sets perfectly clean.
  // The displacement runs down to a residual rag and stays there.
  var INK_SET = 2.2;
  var inkCount = 0;

  function inkBleed(target, animate) {
    var id = "ink-bleed-" + inkCount++;
    var ns = "http://www.w3.org/2000/svg";
    var svg = document.createElementNS(ns, "svg");
    svg.setAttribute("width", "0");
    svg.setAttribute("height", "0");
    svg.setAttribute("aria-hidden", "true");
    svg.style.position = "absolute";

    var filter = document.createElementNS(ns, "filter");
    filter.setAttribute("id", id);
    filter.setAttribute("color-interpolation-filters", "sRGB");

    var turbulence = document.createElementNS(ns, "feTurbulence");
    turbulence.setAttribute("type", "fractalNoise");
    turbulence.setAttribute("baseFrequency", "0.045");
    turbulence.setAttribute("numOctaves", "2");
    turbulence.setAttribute("result", "noise");

    var displace = document.createElementNS(ns, "feDisplacementMap");
    displace.setAttribute("in", "SourceGraphic");
    displace.setAttribute("in2", "noise");
    displace.setAttribute("xChannelSelector", "R");
    displace.setAttribute("yChannelSelector", "G");
    displace.setAttribute("scale", animate ? "14" : String(INK_SET));

    filter.appendChild(turbulence);
    filter.appendChild(displace);
    svg.appendChild(filter);
    document.body.appendChild(svg);

    target.style.filter = "url(#" + id + ")";
    if (!animate) return;

    var start = performance.now();
    var duration = 460;

    (function settle(now) {
      var t = Math.min((now - start) / duration, 1);
      var eased = 1 - Math.pow(1 - t, 3);
      displace.setAttribute("scale", String(INK_SET + (14 - INK_SET) * (1 - eased)));
      if (t < 1) window.requestAnimationFrame(settle);
    })(start);
  }

  // Stamps that are already on the page arrived with the ink long dry.
  function ragStaticStamps() {
    document.querySelectorAll(".report-stamp.is-static").forEach(function (stamp) {
      inkBleed(stamp, false);
    });
  }

  addCopyControls();
  trackReadingPosition();
  stampOnFinish();
  ragStaticStamps();
  rakeLight();
})();
