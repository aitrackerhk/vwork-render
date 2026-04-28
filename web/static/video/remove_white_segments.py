import argparse
import json
import os
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, List, Optional, Tuple


@dataclass(frozen=True)
class Segment:
    start: float
    end: float


def _run_capture(cmd: List[str]) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False)


def _probe(path: Path) -> Tuple[float, bool]:
    cp = _run_capture(
        [
            "ffprobe",
            "-v",
            "error",
            "-print_format",
            "json",
            "-show_format",
            "-show_streams",
            str(path),
        ]
    )
    if cp.returncode != 0:
        raise RuntimeError(f"ffprobe failed for {path}: {cp.stderr.strip()}")

    payload = json.loads(cp.stdout)
    duration_s = float(payload["format"]["duration"])
    has_audio = any((s.get("codec_type") == "audio") for s in payload.get("streams", []))
    return duration_s, has_audio


def _read_exact(stream, n: int) -> Optional[bytes]:
    buf = bytearray()
    while len(buf) < n:
        chunk = stream.read(n - len(buf))
        if not chunk:
            return None
        buf.extend(chunk)
    return bytes(buf)


def _iter_raw_gray_means(
    path: Path, sample_fps: float, w: int, h: int
) -> Iterable[Tuple[float, int, int]]:
    cmd = [
        "ffmpeg",
        "-v",
        "error",
        "-i",
        str(path),
        "-an",
        "-sn",
        "-vf",
        f"fps={sample_fps},scale={w}:{h}:flags=fast_bilinear,format=gray",
        "-f",
        "rawvideo",
        "-pix_fmt",
        "gray",
        "-",
    ]
    p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    assert p.stdout is not None
    frame_size = w * h

    i = 0
    try:
        while True:
            frame = _read_exact(p.stdout, frame_size)
            if frame is None:
                break
            total = 0
            mn = 255
            mx = 0
            for b in frame:
                total += b
                if b < mn:
                    mn = b
                if b > mx:
                    mx = b
            mean = total / frame_size
            yield mean, mn, mx
            i += 1
    except Exception:
        p.kill()
        raise
    finally:
        try:
            p.stdout.close()
        except Exception:
            pass
        rc = p.wait()
        stderr_b = b""
        if p.stderr is not None:
            try:
                stderr_b = p.stderr.read() or b""
            finally:
                try:
                    p.stderr.close()
                except Exception:
                    pass
        if rc != 0:
            raise RuntimeError(f"ffmpeg scan failed for {path}: {stderr_b.decode(errors='replace').strip()}")


def _merge_segments(segments: List[Segment], merge_gap: float) -> List[Segment]:
    if not segments:
        return []
    segments = sorted(segments, key=lambda s: (s.start, s.end))
    out = [segments[0]]
    for s in segments[1:]:
        last = out[-1]
        if s.start <= last.end + merge_gap:
            out[-1] = Segment(start=last.start, end=max(last.end, s.end))
        else:
            out.append(s)
    return out


def _invert_to_keep(duration_s: float, cuts: List[Segment], min_keep_s: float) -> List[Segment]:
    keep: List[Segment] = []
    cursor = 0.0
    for c in cuts:
        if c.start > cursor + min_keep_s:
            keep.append(Segment(cursor, c.start))
        cursor = max(cursor, c.end)
    if duration_s > cursor + min_keep_s:
        keep.append(Segment(cursor, duration_s))
    return keep


def _format_segments(segs: List[Segment]) -> str:
    return ", ".join(f"[{s.start:.3f},{s.end:.3f}]" for s in segs) if segs else "(none)"


def _safe_backup_path(path: Path, backup_ext: str) -> Path:
    candidate = path.with_name(path.name + backup_ext)
    if not candidate.exists():
        return candidate
    for i in range(1, 1000):
        cand = path.with_name(path.name + f"{backup_ext}.{i}")
        if not cand.exists():
            return cand
    raise RuntimeError(f"cannot find backup filename for {path}")


def _build_ffmpeg_concat(
    input_path: Path, keep: List[Segment], has_audio: bool, out_path: Path, crf: int, preset: str
) -> List[str]:
    parts: List[str] = []
    for i, seg in enumerate(keep):
        parts.append(f"[0:v]trim=start={seg.start}:end={seg.end},setpts=PTS-STARTPTS[v{i}]")
        if has_audio:
            parts.append(f"[0:a]atrim=start={seg.start}:end={seg.end},asetpts=PTS-STARTPTS[a{i}]")

    if has_audio:
        concat_inputs = "".join(f"[v{i}][a{i}]" for i in range(len(keep)))
        parts.append(f"{concat_inputs}concat=n={len(keep)}:v=1:a=1[outv][outa]")
    else:
        concat_inputs = "".join(f"[v{i}]" for i in range(len(keep)))
        parts.append(f"{concat_inputs}concat=n={len(keep)}:v=1:a=0[outv]")

    filter_complex = ";".join(parts)

    cmd = [
        "ffmpeg",
        "-v",
        "error",
        "-y",
        "-i",
        str(input_path),
        "-filter_complex",
        filter_complex,
        "-map",
        "[outv]",
        "-c:v",
        "libx264",
        "-preset",
        preset,
        "-crf",
        str(crf),
        "-pix_fmt",
        "yuv420p",
        "-movflags",
        "+faststart",
    ]
    if has_audio:
        cmd += ["-map", "[outa]", "-c:a", "aac", "-b:a", "192k"]
    else:
        cmd += ["-an"]
    cmd += [str(out_path)]
    return cmd


def process_one(
    path: Path,
    sample_fps: float,
    scale: str,
    white_mean: float,
    white_range: int,
    min_white_s: float,
    pad_s: float,
    min_keep_s: float,
    dry_run: bool,
    inplace: bool,
    backup_ext: str,
    suffix: str,
    crf: int,
    preset: str,
) -> bool:
    duration_s, has_audio = _probe(path)
    w_s, h_s = scale.lower().split("x", 1)
    w, h = int(w_s), int(h_s)

    white: List[Segment] = []
    run_start = None
    frame_i = 0
    for mean, mn, mx in _iter_raw_gray_means(path, sample_fps=sample_fps, w=w, h=h):
        is_white = (mean >= white_mean) and ((mx - mn) <= white_range)
        if is_white and run_start is None:
            run_start = frame_i
        if (not is_white) and run_start is not None:
            run_end = frame_i - 1
            start_t = run_start / sample_fps
            end_t = (run_end + 1) / sample_fps
            if end_t - start_t >= min_white_s:
                white.append(
                    Segment(start=max(0.0, start_t - pad_s), end=min(duration_s, end_t + pad_s))
                )
            run_start = None
        frame_i += 1

    if run_start is not None:
        start_t = run_start / sample_fps
        end_t = frame_i / sample_fps
        if end_t - start_t >= min_white_s:
            white.append(Segment(start=max(0.0, start_t - pad_s), end=min(duration_s, end_t + pad_s)))

    white = _merge_segments(white, merge_gap=(1.0 / sample_fps) + pad_s)
    keep = _invert_to_keep(duration_s, white, min_keep_s=min_keep_s)

    if dry_run:
        print(f"{path.name}")
        print(f"  duration={duration_s:.3f}s audio={'yes' if has_audio else 'no'}")
        print(f"  cut={_format_segments(white)}")
        print(f"  keep={_format_segments(keep)}")
        return False

    if not white:
        print(f"{path.name}: no white segments detected, skip")
        return False

    if not keep:
        raise RuntimeError(f"{path.name}: all content detected as white, refusing to output empty video")

    if inplace:
        tmp_out = path.with_name(path.name + ".tmp.clean.mp4")
        out_path = tmp_out
    else:
        out_path = path.with_name(path.stem + suffix + path.suffix)

    cmd = _build_ffmpeg_concat(path, keep=keep, has_audio=has_audio, out_path=out_path, crf=crf, preset=preset)
    cp = _run_capture(cmd)
    if cp.returncode != 0:
        raise RuntimeError(f"ffmpeg concat failed for {path.name}: {cp.stderr.strip()}")

    if inplace:
        backup_path = _safe_backup_path(path, backup_ext=backup_ext)
        shutil.move(str(path), str(backup_path))
        shutil.move(str(tmp_out), str(path))
        print(f"{path.name}: cut={_format_segments(white)} -> replaced (backup: {backup_path.name})")
    else:
        print(f"{path.name}: cut={_format_segments(white)} -> {out_path.name}")

    return True


def main(argv: List[str]) -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--dir", default=".", help="directory containing mp4 files")
    p.add_argument("--glob", default="*.mp4", help="file glob inside --dir")
    p.add_argument("--sample-fps", type=float, default=5.0)
    p.add_argument("--scale", default="64x36")
    p.add_argument("--white-mean", type=float, default=250.0)
    p.add_argument("--white-range", type=int, default=10)
    p.add_argument("--min-white-seconds", type=float, default=0.7)
    p.add_argument("--pad-seconds", type=float, default=0.10)
    p.add_argument("--min-keep-seconds", type=float, default=0.20)
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--inplace", action="store_true")
    p.add_argument("--backup-ext", default=".orig.mp4")
    p.add_argument("--suffix", default=".clean")
    p.add_argument("--crf", type=int, default=23)
    p.add_argument("--preset", default="veryfast")
    args = p.parse_args(argv)

    base = Path(args.dir).resolve()
    files = sorted(base.glob(args.glob))
    if not files:
        print(f"no files matched: {base}\\{args.glob}")
        return 2

    changed = 0
    for f in files:
        if f.name.endswith(args.backup_ext):
            continue
        try:
            did = process_one(
                f,
                sample_fps=args.sample_fps,
                scale=args.scale,
                white_mean=args.white_mean,
                white_range=args.white_range,
                min_white_s=args.min_white_seconds,
                pad_s=args.pad_seconds,
                min_keep_s=args.min_keep_seconds,
                dry_run=args.dry_run,
                inplace=args.inplace,
                backup_ext=args.backup_ext,
                suffix=args.suffix,
                crf=args.crf,
                preset=args.preset,
            )
            if did:
                changed += 1
        except Exception as e:
            print(f"{f.name}: ERROR: {e}", file=sys.stderr)
            return 1

    if args.dry_run:
        return 0

    print(f"done. changed={changed}/{len(files)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
