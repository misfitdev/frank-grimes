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

  // Grimey read this before you did. Each mark is a pen gesture drawn over the
  // thing it is about: geometry is CSS-relative so nothing needs remeasuring.
  var MARKS = {
    // A rough ellipse: one loop, overshot past its own start.
    ellipse: {
      box: "0 0 100 46",
      stretch: true,
      paths: [
        "M92 9C82 2 58 1 38 3 18 5 4 12 5 22c1 10 20 18 44 20 24 2 47-3 48-14 1-9-15-16-34-18"
      ],
      style: "left:-0.45em;right:-0.45em;top:-0.28em;bottom:-0.3em;"
    },
    // A hand underline, laid twice the way a pen doubles back.
    squiggle: {
      box: "0 0 100 12",
      stretch: true,
      paths: [
        "M1 5c14 3 28-1 42 1s28 4 42 1c5-1 10-2 14-4",
        "M4 9c16 2 30-1 45 1s26 3 40 0"
      ],
      // Explicit width: an SVG with a viewBox sizes from its intrinsic ratio
      // when width is auto, so left/right offsets alone will not stretch it.
      style: "left:-0.15em;width:calc(100% + 0.3em);bottom:-0.5em;height:0.62em;"
    },
    // Two strokes and two dots in the margin: this one is a gotcha.
    bang: {
      box: "0 0 26 54",
      paths: ["M8 4C5 18 5 29 7 38", "M20 3c-3 14-3 25-1 34"],
      dots: [
        [7, 47, 2.4],
        [19, 45, 2.4]
      ],
      style: "right:-2.9rem;top:0.6rem;width:1.5rem;height:3.1rem;"
    },
    // A tick where the report was signed off.
    tick: {
      box: "0 0 40 34",
      paths: ["M3 17c4 3 8 7 12 13C21 20 28 9 37 3"],
      style: "left:calc(100% + 0.75rem);top:-0.3rem;width:1.8rem;height:1.55rem;"
    }
  };

  // non-scaling-stroke dashes in screen space, so under a stretched viewBox the
  // path's own length is the wrong unit and the tail never draws. Walk the path
  // through the screen matrix and measure what the browser will actually dash.
  function screenLength(path) {
    var local = path.getTotalLength();
    var ctm = path.getScreenCTM();
    if (!ctm) return local;

    var steps = 24;
    var total = 0;
    var prev = null;

    for (var i = 0; i <= steps; i++) {
      var point = path.getPointAtLength((local * i) / steps).matrixTransform(ctm);
      if (prev) {
        total += Math.hypot(point.x - prev.x, point.y - prev.y);
      }
      prev = point;
    }

    // Sampled chords cut corners; pad so the stroke is never short.
    return total * 1.06;
  }

  function drawMarks() {
    var targets = document.querySelectorAll("[data-mark]");
    if (!targets.length) return;

    var ns = "http://www.w3.org/2000/svg";

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          entry.target.classList.add("is-drawn");
          observer.unobserve(entry.target);
        });
      },
      { rootMargin: "0px 0px -22% 0px" }
    );

    targets.forEach(function (target) {
      var spec = MARKS[target.dataset.mark];
      if (!spec) return;

      var svg = document.createElementNS(ns, "svg");
      svg.setAttribute("class", "pen-mark");
      svg.setAttribute("viewBox", spec.box);
      svg.setAttribute("fill", "none");
      svg.setAttribute("aria-hidden", "true");
      if (spec.stretch) svg.setAttribute("preserveAspectRatio", "none");
      svg.setAttribute("style", spec.style);

      var strokes = [];

      spec.paths.forEach(function (d) {
        var path = document.createElementNS(ns, "path");
        path.setAttribute("d", d);
        svg.appendChild(path);
        strokes.push(path);
      });

      (spec.dots || []).forEach(function (dot) {
        var circle = document.createElementNS(ns, "circle");
        circle.setAttribute("cx", dot[0]);
        circle.setAttribute("cy", dot[1]);
        circle.setAttribute("r", dot[2]);
        circle.setAttribute("class", "pen-dot");
        svg.appendChild(circle);
      });

      target.classList.add("has-mark");
      target.appendChild(svg);

      if (reduced) {
        target.classList.add("is-drawn");
        return;
      }

      // Lengths must be read after the element is in the document. The dash is
      // overshot so the draw always covers the stroke, then dropped once the
      // mark is down: dash units and stretched viewBoxes do not agree, and the
      // finished mark must not depend on them.
      strokes.forEach(function (path, index) {
        var length = screenLength(path) * 1.4;
        var delay = index * 110;

        path.style.strokeDasharray = length;
        path.style.strokeDashoffset = length;
        path.style.transitionDelay = delay + "ms";

        path.addEventListener(
          "transitionend",
          function () {
            path.style.strokeDasharray = "none";
            path.style.strokeDashoffset = "";
          },
          { once: true }
        );
      });

      observer.observe(target);
    });
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
  drawMarks();
  rakeLight();
})();
