import "./style.css";

const GITHUB_OWNER = "yashikota";
const GITHUB_REPO = "hackernews-podcast";
const BRANCH = "podcast-files";
const PUBLIC_DIR = "public";
const HACKERNEWS_JA_BASE_URL = "https://catnose.me/lab/hackernews-ja";

interface GitHubTreeItem {
  path: string;
  type: string;
}

interface GitHubTreeResponse {
  tree: GitHubTreeItem[];
}

interface PodcastEpisode {
  date: string;
  filename: string;
  url: string;
}

async function fetchPodcastList(): Promise<PodcastEpisode[]> {
  const apiUrl = `https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/git/trees/${BRANCH}?recursive=1`;

  const response = await fetch(apiUrl);
  if (!response.ok) {
    throw new Error(`Failed to fetch podcast list: ${response.status}`);
  }

  const data: GitHubTreeResponse = await response.json();

  const episodes: PodcastEpisode[] = data.tree
    .filter(
      (item) =>
        item.type === "blob" &&
        item.path.startsWith(`${PUBLIC_DIR}/`) &&
        item.path.endsWith(".mp3")
    )
    .map((item) => {
      const filename = item.path.replace(`${PUBLIC_DIR}/`, "");
      const dateMatch = filename.match(/hackernews_(\d{4}-\d{2}-\d{2})_podcast\.mp3/);
      const date = dateMatch ? dateMatch[1] : "Unknown";

      return {
        date,
        filename,
        url: `https://raw.githubusercontent.com/${GITHUB_OWNER}/${GITHUB_REPO}/${BRANCH}/${item.path}`,
      };
    })
    .sort((a, b) => b.date.localeCompare(a.date));

  return episodes;
}

function formatDate(dateStr: string): string {
  if (dateStr === "Unknown") return dateStr;
  const date = new Date(dateStr);
  return date.toLocaleDateString("ja-JP", {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "short",
  });
}

function createPlayerUI(episodes: PodcastEpisode[]): string {
  if (episodes.length === 0) {
    return `
      <div class="empty-state">
        <p>まだエピソードがありません</p>
      </div>
    `;
  }

  const latestEpisode = episodes[0];
  const episodeList = episodes
    .map(
      (ep, index) => `
      <li class="episode-item ${index === 0 ? "active" : ""}" data-url="${ep.url}" data-date="${ep.date}" data-index="${index}">
        <a href="${HACKERNEWS_JA_BASE_URL}/${ep.date}" target="_blank" class="episode-link" onclick="event.stopPropagation()">
          <span class="episode-date">${formatDate(ep.date)}</span>
        </a>
        <span class="episode-play-icon">▶</span>
      </li>
    `
    )
    .join("");

  return `
    <header class="header">
      <h1>🎙️ Hacker News 日本語Podcast</h1>
      <p class="subtitle">毎日のHacker Newsニュースを音声でお届け</p>
    </header>

    <main class="player-container">
      <div class="now-playing">
        <span class="now-playing-label">Now Playing</span>
        <h2 class="current-episode-title">
          <a href="${HACKERNEWS_JA_BASE_URL}/${latestEpisode.date}" target="_blank" class="current-episode-link" data-date="${latestEpisode.date}">${formatDate(latestEpisode.date)}</a>
        </h2>
      </div>

      <audio id="audio-player" controls>
        <source src="${latestEpisode.url}" type="audio/mpeg">
        お使いのブラウザは音声再生に対応していません。
      </audio>

      <div class="playback-controls">
        <button id="speed-btn" class="control-btn" title="再生速度">1x</button>
        <button id="skip-back" class="control-btn" title="10秒戻る">⏪ 10s</button>
        <button id="skip-forward" class="control-btn" title="10秒進む">10s ⏩</button>
      </div>
    </main>

    <section class="episode-list-section">
      <h2>📻 エピソード一覧</h2>
      <ul class="episode-list">
        ${episodeList}
      </ul>
    </section>

    <footer class="footer">
      <p>Powered by <a href="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}" target="_blank">hackernews-podcast</a></p>
    </footer>
  `;
}

function getDateFromHash(): string | null {
  const hash = window.location.hash.slice(1);
  if (/^\d{4}-\d{2}-\d{2}$/.test(hash)) {
    return hash;
  }
  return null;
}

function setDateToHash(date: string): void {
  window.history.pushState(null, "", `#${date}`);
}

function setupPlayer(episodes: PodcastEpisode[]): void {
  const audio = document.getElementById("audio-player") as HTMLAudioElement;
  const speedBtn = document.getElementById("speed-btn") as HTMLButtonElement;
  const skipBack = document.getElementById("skip-back") as HTMLButtonElement;
  const skipForward = document.getElementById("skip-forward") as HTMLButtonElement;
  const episodeItems = document.querySelectorAll(".episode-item");
  const currentTitleLink = document.querySelector(".current-episode-link") as HTMLAnchorElement;

  const speeds = [1, 1.25, 1.5, 1.75, 2];
  let speedIndex = 0;
  let currentSpeed = speeds[speedIndex];

  const selectEpisode = (date: string, autoPlay = false) => {
    const episode = episodes.find((ep) => ep.date === date);
    if (!episode) return;

    episodeItems.forEach((ep) => {
      ep.classList.toggle("active", ep.getAttribute("data-date") === date);
    });

    if (currentTitleLink) {
      currentTitleLink.textContent = formatDate(date);
      currentTitleLink.href = `${HACKERNEWS_JA_BASE_URL}/${date}`;
      currentTitleLink.dataset.date = date;
    }

    setDateToHash(date);
    audio.src = episode.url;
    if (autoPlay) audio.play();
  };

  // Handle initial hash or default to latest
  const initialDate = getDateFromHash();
  if (initialDate && episodes.some((ep) => ep.date === initialDate)) {
    selectEpisode(initialDate);
  } else if (episodes.length > 0) {
    setDateToHash(episodes[0].date);
  }

  // Handle browser back/forward
  window.addEventListener("hashchange", () => {
    const date = getDateFromHash();
    if (date) selectEpisode(date);
  });

  speedBtn.addEventListener("click", () => {
    speedIndex = (speedIndex + 1) % speeds.length;
    currentSpeed = speeds[speedIndex];
    audio.playbackRate = currentSpeed;
    speedBtn.textContent = `${currentSpeed}x`;
  });

  skipBack.addEventListener("click", () => {
    audio.currentTime = Math.max(0, audio.currentTime - 10);
  });

  skipForward.addEventListener("click", () => {
    audio.currentTime = Math.min(audio.duration, audio.currentTime + 10);
  });

  audio.addEventListener("loadedmetadata", () => {
    audio.playbackRate = currentSpeed;
  });

  episodeItems.forEach((item) => {
    item.addEventListener("click", (e) => {
      if ((e.target as HTMLElement).closest(".episode-link")) return;

      const date = item.getAttribute("data-date");
      if (date) selectEpisode(date, true);
    });
  });
}

async function init(): Promise<void> {
  const app = document.querySelector<HTMLDivElement>("#app")!;

  app.innerHTML = `
    <div class="loading">
      <p>読み込み中...</p>
    </div>
  `;

  try {
    const episodes = await fetchPodcastList();
    app.innerHTML = createPlayerUI(episodes);
    if (episodes.length > 0) {
      setupPlayer(episodes);
    }
  } catch (error) {
    console.error("Failed to load podcasts:", error);
    app.innerHTML = `
      <div class="error">
        <h2>エラー</h2>
        <p>ポッドキャストの読み込みに失敗しました。</p>
        <p>まだエピソードが公開されていない可能性があります。</p>
      </div>
    `;
  }
}

init();
