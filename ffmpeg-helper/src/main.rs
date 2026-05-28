use anyhow::{bail, Context, Result};
use clap::Parser;
use indicatif::{ProgressBar, ProgressStyle};
use std::path::{Path, PathBuf};
use std::process::Stdio;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio::sync::Semaphore;
use std::sync::Arc;
use std::time::Duration;

/// FFmpeg helper: supports many codecs and formats (h264, h265, vp9, av1, webm, mp4, mkv)
#[derive(Parser, Debug, Clone)]
#[command(author, version, about, long_about = None)]
struct Args {
    /// Input file(s) or directories (can be multiple)
    #[arg(required = true)]
    inputs: Vec<PathBuf>,

    /// Output directory
    #[arg(short, long, default_value = "./out")]
    out: PathBuf,

    /// Video codec to use: copy|h264|h265|vp9|av1|auto
    #[arg(long, default_value = "auto")]
    codec: String,

    /// Output container format: mp4|mkv|webm|mov
    #[arg(long, default_value = "mkv")]
    format: String,

    /// CRF for re-encode (lower = better quality). Values interpreted per codec.
    #[arg(long, default_value_t = 28)]
    crf: u8,

    /// x264/x265 preset (ultrafast, fast, medium, slow…). Default: slow
    #[arg(long, default_value = "slow")]
    preset: String,

    /// Concurrency limit (parallel jobs)
    #[arg(short, long, default_value_t = 2)]
    jobs: usize,

    /// Basic hardware accel: "nvenc" or empty
    #[arg(long, default_value = "")]
    hwaccel: String,

    /// Optimize for size (h264->h265, mp4/mkv->webm)
    #[arg(long, default_value_t = false)]
    optimize: bool,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    // ensure ffmpeg and ffprobe available
    Command::new("ffmpeg")
        .arg("-version")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .await
        .context("ffmpeg not found in PATH")?;

    Command::new("ffprobe")
        .arg("-version")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .await
        .context("ffprobe not found in PATH")?;

    tokio::fs::create_dir_all(&args.out).await?;

    let sem = Arc::new(Semaphore::new(args.jobs));
    let mut tasks = Vec::new();

    for p in expand_inputs(&args.inputs).await? {
        let sem = sem.clone();
        let out_dir = args.out.clone();
        let args = args.clone();
        tasks.push(tokio::spawn(async move {
            let _permit = sem.acquire().await.unwrap();
            if let Err(e) = process_file(&p, &out_dir, &args).await {
                eprintln!("✖ {}: {:#}", p.display(), e);
            }
        }));
    }

    for t in tasks {
        let _ = t.await;
    }

    Ok(())
}

async fn expand_inputs(inputs: &[PathBuf]) -> Result<Vec<PathBuf>> {
    let mut out = Vec::new();
    for p in inputs {
        if p.is_dir() {
            let mut dir = tokio::fs::read_dir(p).await?;
            while let Some(entry) = dir.next_entry().await? {
                let path = entry.path();
                if is_media(&path) {
                    out.push(path);
                }
            }
        } else {
            out.push(p.clone());
        }
    }
    Ok(out)
}

fn is_media(p: &Path) -> bool {
    match p.extension().and_then(|s| s.to_str()) {
        Some(ext) => matches!(
            ext.to_lowercase().as_str(),
            "mp4" | "mov" | "mkv" | "avi" | "webm" | "m4v"
        ),
        None => false,
    }
}

async fn get_duration_seconds(path: &Path) -> Result<f64> {
    let out = Command::new("ffprobe")
        .args([
            "-v",
            "error",
            "-show_entries",
            "format=duration",
            "-of",
            "default=noprint_wrappers=1:nokey=1",
        ])
        .arg(path)
        .output()
        .await?;
    if !out.status.success() {
        bail!("ffprobe failed for {}", path.display());
    }
    let s = String::from_utf8_lossy(&out.stdout).trim().to_string();
    let dur = s.parse::<f64>().context("parsing duration")?;
    Ok(dur)
}

async fn get_video_codec_name(path: &Path) -> Result<Option<String>> {
    let out = Command::new("ffprobe")
        .args([
            "-v",
            "error",
            "-select_streams",
            "v:0",
            "-show_entries",
            "stream=codec_name",
            "-of",
            "default=noprint_wrappers=1:nokey=1",
        ])
        .arg(path)
        .output()
        .await?;
    if !out.status.success() {
        return Ok(None);
    }
    let s = String::from_utf8_lossy(&out.stdout).trim().to_string();
    if s.is_empty() {
        Ok(None)
    } else {
        Ok(Some(s))
    }
}

fn choose_audio_codec(format: &str) -> &'static str {
    match format {
        "webm" => "libopus",
        "mp4" => "aac",
        _ => "copy",
    }
}

fn output_ext(format: &str) -> &'static str {
    match format {
        "mp4" => "mp4",
        "webm" => "webm",
        "mov" => "mov",
        _ => "mkv",
    }
}

fn build_codec_args(codec: &str, format: &str, args: &Args) -> Vec<String> {
    let mut v = Vec::new();
    match codec {
        "copy" => {
            v.push("-c".to_string());
            v.push("copy".to_string());
        }
        "h264" => {
            if !args.hwaccel.is_empty() && args.hwaccel == "nvenc" {
                v.push("-c:v".to_string());
                v.push("h264_nvenc".to_string());
            } else {
                v.push("-c:v".to_string());
                v.push("libx264".to_string());
                v.push("-preset".to_string());
                v.push(args.preset.clone());
                v.push("-crf".to_string());
                v.push(args.crf.to_string());
            }
            v.push("-c:a".to_string());
            v.push(choose_audio_codec(format).to_string());
        }
        "h265" => {
            if !args.hwaccel.is_empty() && args.hwaccel == "nvenc" {
                v.push("-c:v".to_string());
                v.push("hevc_nvenc".to_string());
            } else {
                v.push("-c:v".to_string());
                v.push("libx265".to_string());
                v.push("-preset".to_string());
                v.push(args.preset.clone());
                v.push("-crf".to_string());
                v.push(args.crf.to_string());
            }
            v.push("-c:a".to_string());
            v.push(choose_audio_codec(format).to_string());
        }
        "vp9" => {
            v.push("-c:v".to_string());
            v.push("libvpx-vp9".to_string());
            v.push("-b:v".to_string());
            v.push("0".to_string());
            v.push("-crf".to_string());
            v.push(args.crf.to_string());
            v.push("-c:a".to_string());
            // webm friendly audio
            v.push("libopus".to_string());
        }
        "av1" => {
            v.push("-c:v".to_string());
            v.push("libaom-av1".to_string());
            v.push("-crf".to_string());
            v.push(args.crf.to_string());
            v.push("-b:v".to_string());
            v.push("0".to_string());
            v.push("-c:a".to_string());
            v.push("libopus".to_string());
        }
        _ => {
            // auto: choose by format
            match format {
                "webm" => return build_codec_args("vp9", format, args),
                "mp4" | "mov" => return build_codec_args("h265", format, args),
                _ => return build_codec_args("h265", format, args),
            }
        }
    }
    v
}

async fn process_file(input: &Path, out_dir: &Path, args: &Args) -> Result<()> {
    let stem = input
        .file_stem()
        .map(|s| s.to_string_lossy().into_owned())
        .unwrap_or_else(|| input.file_name().and_then(|n| n.to_str()).unwrap_or("output").to_string());

    // Determine target format and codec per-file (optimize assists)
    let mut target_format = args.format.clone();
    let mut target_codec = args.codec.clone();

    if args.optimize {
        if let Some(ext_) = input.extension().and_then(|s| s.to_str()).map(|s| s.to_lowercase()) {
            if ext_ == "mp4" || ext_ == "mkv" {
                target_format = "webm".to_string();
                if args.codec == "auto" {
                    target_codec = "vp9".to_string();
                }
            }
        }
        if let Some(in_codec) = get_video_codec_name(input).await? {
            if in_codec == "h264" && args.codec == "auto" && target_codec == "auto" {
                target_codec = "h265".to_string();
            }
        }
    }

    let ext = output_ext(&target_format);
    let out_path = out_dir.join(format!("{}.{}", stem, ext));

    let duration = get_duration_seconds(input).await.unwrap_or(0.0);
    let display_name = input
        .file_name()
        .map(|s| s.to_string_lossy().into_owned())
        .unwrap_or_else(|| input.display().to_string());

    let pb = if duration > 0.0 {
        let pb = ProgressBar::new((duration * 1000.0) as u64);
        pb.set_style(
            ProgressStyle::with_template("{spinner:.green} {msg} [{wide_bar:.cyan/blue}] {percent}%")
                .unwrap()
                .progress_chars("=> "),
        );
        pb.set_message(display_name.clone());
        pb
    } else {
        let pb = ProgressBar::new_spinner();
        pb.set_message(display_name.clone());
        pb.enable_steady_tick(Duration::from_millis(100));
        pb
    };

    let codec = target_codec.as_str();
    let mut cmd = Command::new("ffmpeg");
    cmd.arg("-y")
        .arg("-hide_banner")
        .arg("-loglevel")
        .arg("error")
        .arg("-progress")
        .arg("pipe:1")
        .arg("-nostats")
        .arg("-i")
        .arg(input);

    let codec_args = build_codec_args(codec, &target_format, args);
    for a in codec_args {
        cmd.arg(a);
    }

    cmd.arg("-threads").arg("0");

    cmd.arg(out_path.to_string_lossy().to_string());
    cmd.stdout(Stdio::piped());
    cmd.stderr(Stdio::inherit()); // show errors

    let mut child = cmd.spawn().context("spawn ffmpeg")?;
    if let Some(stdout) = child.stdout.take() {
        let mut reader = BufReader::new(stdout).lines();
        while let Ok(Some(line)) = reader.next_line().await {
            // ffmpeg -progress writes key=value lines
            if line.starts_with("out_time_ms=") {
                if let Ok(ms) = line["out_time_ms=".len()..].parse::<u64>() {
                    let progressed = ms;
                    pb.set_position(progressed);
                }
            } else if line.starts_with("progress=end") {
                pb.finish_with_message(format!("done {}", display_name));
            }
        }
    }

    let status = child.wait().await?;
    if !status.success() {
        bail!("ffmpeg failed for {}", input.display());
    }

    if duration > 0.0 {
        pb.finish_with_message(format!("finished {}", display_name));
    } else {
        pb.finish();
    }

    Ok(())
}
