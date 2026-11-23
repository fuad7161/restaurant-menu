# The 20-second cartoon video

`scenes.html` is the whole video: five scenes on one 20-second timeline, drawn with HTML, CSS and
inline SVG, using the app's own Bangla font. `render.mjs` steps through it frame by frame in
headless Firefox, `sound.py` makes the soundtrack, and ffmpeg joins both into `../bosho-20s.mp4`
(1080 × 1920, 30 fps). The MP4 is not tracked by git (see `.gitignore`); rebuild it with the steps below.

The soundtrack is synthesised by `sound.py`, so it is free of copyright: slow, slightly tense music
while the guest waits (0–7 s), upbeat music from the QR scan on (7–18 s) and a bright final chord,
plus sound effects on the animation's beats: bubble pops, a ticking clock, the scan sweep and
beep, the tap and ding when a dish is added, the kitchen bell, rising notes with the sales bars
and a chime on the logo. It has no voice.

| Time | Scene | On screen |
| --- | --- | --- |
| 0–3 s | Hook | A guest waves, the waiter walks away: "ভাই, অর্ডারটা নেবেন?", "আর কতক্ষণ বসে থাকব?", "চেনা দৃশ্য?" |
| 3–7 s | Problem | A spinning clock and three waits: ordering, asking about the food, asking for the bill |
| 7–14 s | Solution | The table QR card, the phone scans it, the menu opens, a dish is added, the order cooks with a progress bar; "কোনো অ্যাপ লাগবে না" |
| 14–18 s | Owner | Orders arrive on the kitchen tablet, the owner smiles at the day's sales |
| 18–20 s | End card | বসো · "বসুন · স্ক্যান করুন · অর্ডার দিন" · "আজই বিনামূল্যে শুরু করুন" · bosho.food |

The numbers shown (prices, "৳১২,৪৫০") are illustrations, not real data. The end card shows `bosho.food`, the current address;
change it if the domain changes.

## Watch it play

From the repo root:

```sh
python3 -m http.server 18090 --bind 127.0.0.1
# open http://127.0.0.1:18090/docs/marketing/video/scenes.html (set the browser window to a phone size)
```

## Render the MP4 again (after changing a scene)

Needs Firefox, Node 22 or newer, Python 3 with numpy, and ffmpeg.

```sh
# 1. serve the repo root (the page loads the fonts from web/static/fonts)
python3 -m http.server 18090 --bind 127.0.0.1 &

# 2. start headless Firefox with WebDriver BiDi on port 19222
firefox --headless --no-remote --profile "$(mktemp -d)" --remote-debugging-port 19222 about:blank &

# 3. render 600 frames (about 3 minutes)
node docs/marketing/video/render.mjs http://127.0.0.1:18090/docs/marketing/video/scenes.html /tmp/bosho-frames 30 20

# 4. make the soundtrack (change sound.py to move or change an effect)
python3 docs/marketing/video/sound.py /tmp/bosho-sound.wav

# 5. encode, with the sound at the usual loudness for social media (-16 LUFS)
ffmpeg -y -framerate 30 -i /tmp/bosho-frames/%04d.png -i /tmp/bosho-sound.wav \
  -c:v libx264 -preset slow -crf 20 -pix_fmt yuv420p \
  -af "loudnorm=I=-16:TP=-1.5:LRA=11" -ar 48000 -c:a aac -b:a 160k -movflags +faststart -shortest \
  docs/marketing/bosho-20s.mp4
```

If you move a scene in `scenes.html`, move its effects in `sound.py` by the same amount (the times
in both files are in seconds from the start).

For a quick check, render 2 frames a second instead (`... /tmp/bosho-preview 2 20`) and look at
the PNGs.

## Adding the voice

The video has music and effects but no voice. Record the script in `../video-script-20s.md` in your
own voice (a phone is fine, in a quiet room) and put it over the video in CapCut or any editor,
turning the music down a little under the voice. Each scene's text is on screen, so the video also
works with the sound off.
