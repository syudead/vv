// 閲覧サイトの小さな振る舞い: 検索、配色、目次の現在位置、狭い画面の一覧。
(function () {
  "use strict";
  var body = document.body;
  var root = body.dataset.root || "";
  var input = document.getElementById("search");
  var results = document.getElementById("results");
  var tree = document.getElementById("tree");

  // 配色: 既定は OS に従い、ボタンで明暗を固定する。
  document.querySelector(".theme").addEventListener("click", function () {
    var el = document.documentElement;
    var dark = el.dataset.theme
      ? el.dataset.theme === "dark"
      : matchMedia("(prefers-color-scheme: dark)").matches;
    el.dataset.theme = dark ? "light" : "dark";
    try { localStorage.setItem("docsite-theme", el.dataset.theme); } catch (e) {}
  });

  // 狭い画面の一覧の開け閉め。
  var menu = document.querySelector(".menu");
  menu.addEventListener("click", function () {
    var open = body.classList.toggle("nav-open");
    menu.setAttribute("aria-expanded", String(open));
  });

  // 今の文書を一覧の見える位置へ。scrollIntoView はページ全体まで動かすので、
  // 一覧の中だけを動かす。
  var sidebar = document.getElementById("sidebar");
  var current = tree.querySelector('[aria-current="page"]');
  if (current) {
    var offset = current.getBoundingClientRect().top - sidebar.getBoundingClientRect().top;
    sidebar.scrollTop = offset - sidebar.clientHeight / 2;
  }

  // 見出しの横に # を置き、その節への URL を取れるようにする。
  document.querySelectorAll(".markdown :is(h2, h3, h4)[id]").forEach(function (h) {
    var a = document.createElement("a");
    a.className = "heading-anchor";
    a.href = "#" + h.id;
    a.textContent = "#";
    a.setAttribute("aria-label", "この節へのリンク");
    h.appendChild(a);
  });

  // 目次: 今読んでいる節を示す。
  var tocLinks = Array.prototype.slice.call(document.querySelectorAll(".toc a"));
  if (tocLinks.length && "IntersectionObserver" in window) {
    var byId = {};
    tocLinks.forEach(function (a) { byId[decodeURIComponent(a.hash.slice(1))] = a; });
    var visible = new Set();
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) visible.add(e.target.id); else visible.delete(e.target.id);
      });
      var first = tocLinks.find(function (a) { return visible.has(decodeURIComponent(a.hash.slice(1))); });
      if (!first) return;
      tocLinks.forEach(function (a) { a.classList.toggle("active", a === first); });
    }, { rootMargin: "-60px 0px -70% 0px" });
    Object.keys(byId).forEach(function (id) {
      var h = document.getElementById(id);
      if (h) observer.observe(h);
    });
  }

  // 検索: 索引は初めて入力したときに読む。
  var loading = null;
  function loadIndex() {
    if (window.DOCSITE_INDEX) return Promise.resolve(window.DOCSITE_INDEX);
    if (!loading) {
      loading = new Promise(function (resolve, reject) {
        var s = document.createElement("script");
        s.src = root + "_docsite/search-index.js";
        s.onload = function () { resolve(window.DOCSITE_INDEX || []); };
        s.onerror = function (e) {
          // 失敗を覚えたままにせず、次の入力で読み直せるようにする。
          s.remove();
          loading = null;
          reject(e);
        };
        document.head.appendChild(s);
      });
    }
    return loading;
  }

  function escapeHTML(s) {
    return s.replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  // highlight は元の文字列の上で全ての語の一致範囲を集めて重なりをまとめ、
  // 範囲ごとに1度だけエスケープして <mark> で囲む。置換を語ごとに重ねると、
  // 先に入れた <mark> の中身まで次の語で置き換えてしまう。
  function highlight(s, terms) {
    var lower = s.toLowerCase();
    var ranges = [];
    terms.forEach(function (t) {
      for (var i = lower.indexOf(t); t && i >= 0; i = lower.indexOf(t, i + t.length)) {
        ranges.push([i, i + t.length]);
      }
    });
    ranges.sort(function (a, b) { return a[0] - b[0]; });
    var out = "";
    var pos = 0;
    ranges.forEach(function (r) {
      if (r[1] <= pos) return;
      var start = Math.max(r[0], pos);
      out += escapeHTML(s.slice(pos, start)) + "<mark>" + escapeHTML(s.slice(start, r[1])) + "</mark>";
      pos = r[1];
    });
    return out + escapeHTML(s.slice(pos));
  }

  function snippet(text, term) {
    var i = text.toLowerCase().indexOf(term);
    if (i < 0) return "";
    var start = Math.max(0, i - 30);
    return (start > 0 ? "…" : "") + text.slice(start, i + term.length + 60) + "…";
  }

  function search(query) {
    var terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (!terms.length) {
      results.hidden = true;
      tree.hidden = false;
      return;
    }
    loadIndex().then(function (index) {
      if (input.value.toLowerCase().split(/\s+/).filter(Boolean).join(" ") !== terms.join(" ")) return;
      var hits = [];
      index.forEach(function (doc) {
        var head = (doc.t + " " + doc.p).toLowerCase();
        var text = doc.b.toLowerCase();
        var score = 0;
        for (var i = 0; i < terms.length; i++) {
          var t = terms[i];
          if (head.indexOf(t) >= 0) score += 10;
          else if (text.indexOf(t) >= 0) score += 1;
          else return;
        }
        hits.push({ doc: doc, score: score });
      });
      hits.sort(function (a, b) { return b.score - a.score || a.doc.p.localeCompare(b.doc.p); });
      results.innerHTML = hits.length
        ? hits.slice(0, 60).map(function (h) {
            var d = h.doc;
            var sn = snippet(d.b, terms[0]);
            return '<li><a href="' + root + encodeURI(d.u) + '">' + highlight(d.t, terms) +
              "<small>" + escapeHTML(d.p) + "</small>" +
              (sn ? '<span class="snippet">' + highlight(sn, terms) + "</span>" : "") + "</a></li>";
          }).join("")
        : '<li class="empty">「' + escapeHTML(query) + "」に合う文書はありません</li>";
      results.hidden = false;
      tree.hidden = true;
    }, function () {
      results.innerHTML = '<li class="empty">検索の索引を読めませんでした</li>';
      results.hidden = false;
    });
  }

  input.addEventListener("input", function () { search(input.value); });
  input.addEventListener("focus", function () { loadIndex().catch(function () {}); });
  input.addEventListener("keydown", function (e) {
    if (e.key === "Escape") {
      input.value = "";
      search("");
      input.blur();
    } else if (e.key === "Enter") {
      var first = results.querySelector("a");
      if (first) location.href = first.href;
    }
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "/" && document.activeElement !== input && !/INPUT|TEXTAREA/.test(document.activeElement.tagName)) {
      e.preventDefault();
      input.focus();
      if (window.innerWidth <= 820) body.classList.add("nav-open");
    }
  });
  input.addEventListener("focus", function () {
    if (window.innerWidth <= 820) body.classList.add("nav-open");
  });
})();
