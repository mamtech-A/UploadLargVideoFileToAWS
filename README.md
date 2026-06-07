# UploadLargeVideo

Fault-tolerant video uploader to AWS S3 using multipart upload. Designed for mobile environments with unreliable connectivity — if the upload is interrupted, re-running the same command resumes from where it stopped.

## Docker (recommended)

No Go installation needed. Works on any device with Docker.

### 1. Set up credentials

```bash
cp .env.example .env
```

Edit `.env` with your AWS credentials:

```env
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=us-east-1
```

### 2. Build the image

```bash
docker compose build
```

### 3. Upload a video

Place your video inside the `Video/` folder, then run:

```bash
docker compose run --rm uploader \
  -file /videos/your-video.mp4 \
  -bucket your-bucket-name \
  -key path/in/s3/your-video.mp4 \
  -part-size-mb 8 \
  -retries 5
```

The `Video/` folder is mounted as `/videos` inside the container.

---

## Build from source

```bash
go build -o UploadLargeVideo .
```

## Usage (without Docker)

```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=eu-central-1

./UploadLargeVideo \
  -file /path/to/video.mp4 \
  -bucket your-bucket-name \
  -key path/in/s3/video.mp4 \
  -part-size-mb 8 \
  -retries 5
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-file` | required | Local path to the video file |
| `-bucket` | required | S3 bucket name |
| `-key` | required | S3 object key (destination path) |
| `-part-size-mb` | `8` | Size of each upload chunk in MB (minimum 5) |
| `-retries` | `5` | Max retries per chunk on network error |
| `-region` | | AWS region (overrides `AWS_REGION` env var) |

## Resume after interruption

If the upload is interrupted (Ctrl+C, lost signal, crash), a state file is saved next to the video:

```
/path/to/video.mp4.upload-state.json
```

Re-run the exact same command to resume. Already-uploaded parts are skipped automatically.

## IAM permissions required

The AWS user needs the following S3 actions on the target bucket:

```json
{
  "Effect": "Allow",
  "Action": [
    "s3:PutObject",
    "s3:CreateMultipartUpload",
    "s3:UploadPart",
    "s3:CompleteMultipartUpload",
    "s3:AbortMultipartUpload"
  ],
  "Resource": "arn:aws:s3:::your-bucket-name/*"
}
```
