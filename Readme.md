Compressor is a lightweight folder watcher that transcodes videos with `ffmpeg`.

## Features

- Watches a single level input directory and schedules new files for compression.
- Renames files with a `.processing` suffix to coordinate multiple replicas.
- Runs the configured `ffmpeg` command ( GPU friendly defaults supplied ).
- Writes results into an output directory and optionally deletes sources.
- Exposes a `/status` endpoint that returns HTTP 200 for health checks.

## Configuration

Environment variables drive the runtime configuration. Defaults are shown in parentheses.

| Variable                                                            | Description                                                                                                                                                                                                                                                                                                                                                                           |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `INPUT_DIR` (`/input`)                                              | Directory to scan and watch for new videos.                                                                                                                                                                                                                                                                                                                                           |
| `OUTPUT_DIR` (`/output`)                                            | Directory where encoded files are written.                                                                                                                                                                                                                                                                                                                                            |
| `VIDEO_EXTENSIONS` (`.mp4,.mkv,.mov,.avi,.flv,.wmv,.m4v,.webm,.ts`) | Comma separated list of extensions that should be processed.                                                                                                                                                                                                                                                                                                                          |
| `FFMPEG_BIN` (`ffmpeg`)                                             | Binary to invoke.                                                                                                                                                                                                                                                                                                                                                                     |
| `FFMPEG_COMMAND`                                                    | Arguments passed to `ffmpeg`. Must include `{{input}}` and `{{output}}` placeholders. Default: `-y -hwaccel cuda -hwaccel_device 0 -i {{input}} -c:v hevc_nvenc -qp 25 -preset p6 -gpu 0 -b_qfactor 1.1 -b_ref_mode middle -bf 3 -g 250 -i_qfactor 0.75 -max_muxing_queue_size 1024 -multipass 1 -rc vbr -rc-lookahead 20 -temporal-aq 1 -tune hq -c:a aac -af volume=2.0 {{output}}` |
| `FFMPEG_COMMAND_CPU`                                                | CPU fallback arguments if GPU not detected. Default: `-y -i {{input}} -c:v libx264 -preset slow -crf 22 -c:a aac {{output}}`                                                                                                                                                                                                                                                          |
| `OUTPUT_EXTENSION` (`.mp4`)                                         | Extension applied to the output file name.                                                                                                                                                                                                                                                                                                                                            |
| `DELETE_SOURCE` (`false`)                                           | When `true`, removes the processed input file instead of restoring it.                                                                                                                                                                                                                                                                                                                |
| `PROCESSING_SUFFIX` (`.processing`)                                 | Suffix appended while a file is in flight.                                                                                                                                                                                                                                                                                                                                            |
| `MAX_CONCURRENT` (`1`)                                              | Number of concurrent transcodes. Consider GPU capacity when raising.                                                                                                                                                                                                                                                                                                                  |
| `QUEUE_SIZE` (`128`)                                                | Work queue buffer length.                                                                                                                                                                                                                                                                                                                                                             |
| `FILE_STABILITY_DURATION` (`3s`)                                    | How long a file size must remain unchanged before processing.                                                                                                                                                                                                                                                                                                                         |
| `RESCAN_INTERVAL` (`30s`)                                           | Periodic full directory rescan interval.                                                                                                                                                                                                                                                                                                                                              |
| `PORT` (`8080`)                                                     | Port for the HTTP `/status` endpoint.                                                                                                                                                                                                                                                                                                                                                 |

Placeholders are shell escaped before the command line is parsed, so paths containing spaces are handled safely.

## Running Locally

```bash
go build -o compressor ./cmd/compressor
INPUT_DIR=$(pwd)/test_input OUTPUT_DIR=$(pwd)/test_output ./compressor
```

Place a video file in `test_input/` and watch it get processed to `test_output/`. The test directories are gitignored.

## Container Notes

- Mount the hot folder to `/input` and the destination to `/output`.
- Expose the health endpoint through your orchestrator: `http://<host>:8080/status`.
- Provide GPU-capable `ffmpeg` binaries in the image (for example via CUDA base images).

## Docker

Build and run locally with GPU:

```bash
docker build -t compressor .
docker run --gpus all -v $(pwd)/test_input:/input -v $(pwd)/test_output:/output -p 8080:8080 compressor
```

## Docker Compose

```bash
docker-compose up --build
```

## Kubernetes

Apply the YAMLs:

```bash
kubectl apply -f k8s/
```

This creates PVCs for input/output volumes. Mount your persistent volumes accordingly. The deployment requests 1 GPU.

## Health Endpoint

- `GET /status` → `200 OK` with body `ok`

The watcher logs every successful encode with source and destination paths.
