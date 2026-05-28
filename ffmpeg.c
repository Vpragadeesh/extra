#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <sys/time.h>
#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavfilter/avfilter.h>
#include <libavutil/opt.h>
#include <libavutil/imgutils.h>
#include <libswscale/swscale.h>
#include <libswresample/swresample.h>
#include <libavutil/hwcontext.h>

// Progress bar structure
typedef struct {
    int64_t total_duration;
    int64_t current_pts;
    time_t start_time;
    int bar_width;
} ProgressContext;

void init_progress(ProgressContext *ctx, int64_t duration) {
    ctx->total_duration = duration;
    ctx->current_pts = 0;
    ctx->start_time = time(NULL);
    ctx->bar_width = 50;
}

void update_progress(ProgressContext *ctx, int64_t pts, AVRational time_base) {
    if (ctx->total_duration <= 0) return;

    double current_time = pts * av_q2d(time_base);
    double total_time = ctx->total_duration / (double)AV_TIME_BASE;
    double progress = current_time / total_time;
    if (progress > 1.0) progress = 1.0;
    if (progress < 0.0) progress = 0.0;

    time_t now = time(NULL);
    double elapsed = difftime(now, ctx->start_time);
    double eta = 0;
    if (progress > 0.01) {
        eta = (elapsed / progress) - elapsed;
    }

    int filled = (int)(progress * ctx->bar_width);

    printf("\r[");
    for (int i = 0; i < ctx->bar_width; i++) {
        if (i < filled) printf("█");
        else printf("░");
    }

    int eta_hours = (int)(eta / 3600);
    int eta_mins = (int)((eta - eta_hours * 3600) / 60);
    int eta_secs = (int)(eta - eta_hours * 3600 - eta_mins * 60);

    int cur_hours = (int)(current_time / 3600);
    int cur_mins = (int)((current_time - cur_hours * 3600) / 60);
    int cur_secs = (int)(current_time - cur_hours * 3600 - cur_mins * 60);

    int tot_hours = (int)(total_time / 3600);
    int tot_mins = (int)((total_time - tot_hours * 3600) / 60);
    int tot_secs = (int)(total_time - tot_hours * 3600 - tot_mins * 60);

    printf("] %5.1f%% | %02d:%02d:%02d / %02d:%02d:%02d | ETA: %02d:%02d:%02d ",
           progress * 100.0,
           cur_hours, cur_mins, cur_secs,
           tot_hours, tot_mins, tot_secs,
           eta_hours, eta_mins, eta_secs);
    fflush(stdout);
}

void finish_progress() {
    printf("\n");
}

void print_usage() {
    printf("Usage: ffmpeg_tool <operation> [options]\n");
    printf("Operations:\n");
    printf("  info <input>\n");
    printf("  convert <input> <output> [vcodec] [bitrate] [resolution] [fps]\n");
    printf("  extract_audio <input> <output> [acodec]\n");
    printf("  cut <input> <output> <start> <duration>\n");
    printf("  merge <output> <input1> <input2> ...\n");
    printf("  add_filter <input> <output> <filter>\n");
    // Add more as needed
}

int convert_video(const char *input, const char *output, const char *vcodec, const char *abitrate, const char *resolution, const char *fps) {
    AVFormatContext *input_ctx = NULL;
    AVFormatContext *output_ctx = NULL;
    AVCodec *decoder = NULL;
    AVCodec *encoder = NULL;
    AVCodecContext *dec_ctx = NULL;
    AVCodecContext *enc_ctx = NULL;
    AVFrame *frame = NULL;
    AVPacket *packet = NULL;
    int ret;
    enum AVHWDeviceType hw_type = av_hwdevice_find_type_by_name("cuda");
    AVBufferRef *hw_device_ctx = NULL;
    if (hw_type != AV_HWDEVICE_TYPE_NONE) {
        ret = av_hwdevice_ctx_create(&hw_device_ctx, hw_type, NULL, NULL, 0);
        if (ret < 0) {
            hw_device_ctx = NULL;
        }
    }
    ret = avformat_open_input(&input_ctx, input, NULL, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot open input file\n");
        return ret;
    }
    ret = avformat_find_stream_info(input_ctx, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot find stream info\n");
        return ret;
    }
    int video_stream_index = -1;
    for (int i = 0; i < input_ctx->nb_streams; i++) {
        if (input_ctx->streams[i]->codecpar->codec_type == AVMEDIA_TYPE_VIDEO) {
            video_stream_index = i;
            break;
        }
    }
    if (video_stream_index == -1) {
        fprintf(stderr, "No video stream found\n");
        return -1;
    }
    decoder = avcodec_find_decoder(input_ctx->streams[video_stream_index]->codecpar->codec_id);
    if (!decoder) {
        fprintf(stderr, "Decoder not found\n");
        return -1;
    }
    dec_ctx = avcodec_alloc_context3(decoder);
    if (!dec_ctx) {
        fprintf(stderr, "Cannot alloc decoder context\n");
        return -1;
    }
    ret = avcodec_parameters_to_context(dec_ctx, input_ctx->streams[video_stream_index]->codecpar);
    if (ret < 0) {
        fprintf(stderr, "Cannot copy parameters\n");
        return ret;
    }
    ret = avcodec_open2(dec_ctx, decoder, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot open decoder\n");
        return ret;
    }
    if (hw_device_ctx && strcmp(vcodec, "h264") == 0) {
        encoder = avcodec_find_encoder_by_name("h264_nvenc");
    } else if (hw_device_ctx && (strcmp(vcodec, "hevc") == 0 || strcmp(vcodec, "h265") == 0)) {
        encoder = avcodec_find_encoder_by_name("hevc_nvenc");
    } else {
        if (strcmp(vcodec, "h265") == 0) {
            encoder = avcodec_find_encoder_by_name("libx265");
        } else {
            encoder = avcodec_find_encoder_by_name(vcodec);
        }
    }
    if (!encoder) {
        fprintf(stderr, "Encoder not found\n");
        return -1;
    }
    enc_ctx = avcodec_alloc_context3(encoder);
    if (!enc_ctx) {
        fprintf(stderr, "Cannot alloc encoder context\n");
        return -1;
    }
    if (hw_device_ctx) {
        enc_ctx->hw_device_ctx = av_buffer_ref(hw_device_ctx);
    }
    enc_ctx->width = dec_ctx->width;
    enc_ctx->height = dec_ctx->height;
    if (fps) {
        enc_ctx->framerate = av_make_q(atoi(fps), 1);
    } else {
        enc_ctx->framerate = dec_ctx->framerate;
        if (enc_ctx->framerate.num == 0) {
            enc_ctx->framerate = av_make_q(30, 1);
        }
    }
    enc_ctx->time_base = av_inv_q(enc_ctx->framerate);
    enc_ctx->pix_fmt = encoder->pix_fmts ? encoder->pix_fmts[0] : AV_PIX_FMT_YUV420P;
    ret = avcodec_open2(enc_ctx, encoder, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot open encoder\n");
        return ret;
    }
    ret = avformat_alloc_output_context2(&output_ctx, NULL, NULL, output);
    if (ret < 0) {
        fprintf(stderr, "Cannot alloc output context\n");
        return ret;
    }
    AVStream *out_stream = avformat_new_stream(output_ctx, NULL);
    if (!out_stream) {
        fprintf(stderr, "Cannot create output stream\n");
        return -1;
    }
    ret = avcodec_parameters_from_context(out_stream->codecpar, enc_ctx);
    if (ret < 0) {
        fprintf(stderr, "Cannot copy parameters to stream\n");
        return ret;
    }
    // Create streams for other inputs
    for (int i = 0; i < input_ctx->nb_streams; i++) {
        if (i != video_stream_index) {
            AVStream *other_out_stream = avformat_new_stream(output_ctx, NULL);
            if (!other_out_stream) {
                fprintf(stderr, "Cannot create output stream\n");
                return -1;
            }
            ret = avcodec_parameters_copy(other_out_stream->codecpar, input_ctx->streams[i]->codecpar);
            if (ret < 0) {
                fprintf(stderr, "Cannot copy parameters\n");
                return ret;
            }
            other_out_stream->codecpar->codec_tag = 0;
        }
    }
    if (!(output_ctx->oformat->flags & AVFMT_NOFILE)) {
        ret = avio_open(&output_ctx->pb, output, AVIO_FLAG_WRITE);
        if (ret < 0) {
            fprintf(stderr, "Cannot open output file\n");
            return ret;
        }
    }
    ret = avformat_write_header(output_ctx, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot write header\n");
        return ret;
    }

    // Initialize progress bar
    ProgressContext progress;
    init_progress(&progress, input_ctx->duration);

    frame = av_frame_alloc();
    packet = av_packet_alloc();
    while (1) {
        ret = av_read_frame(input_ctx, packet);
        if (ret < 0) break;
        if (packet->stream_index == video_stream_index) {
            // Update progress
            update_progress(&progress, packet->pts, input_ctx->streams[video_stream_index]->time_base);

            ret = avcodec_send_packet(dec_ctx, packet);
            if (ret < 0) {
                fprintf(stderr, "Error sending packet to decoder\n");
                break;
            }
            while (ret >= 0) {
                ret = avcodec_receive_frame(dec_ctx, frame);
                if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) break;
                if (ret < 0) {
                    fprintf(stderr, "Error receiving frame from decoder\n");
                    break;
                }
                ret = avcodec_send_frame(enc_ctx, frame);
                if (ret < 0) {
                    fprintf(stderr, "Error sending frame to encoder\n");
                    break;
                }
                while (ret >= 0) {
                    AVPacket *enc_packet = av_packet_alloc();
                    ret = avcodec_receive_packet(enc_ctx, enc_packet);
                    if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) break;
                    if (ret < 0) {
                        fprintf(stderr, "Error receiving packet from encoder\n");
                        break;
                    }
                    enc_packet->stream_index = 0;
                    av_packet_rescale_ts(enc_packet, enc_ctx->time_base, out_stream->time_base);
                    ret = av_interleaved_write_frame(output_ctx, enc_packet);
                    av_packet_free(&enc_packet);
                }
            }
        } else {
            // Copy other streams
            int out_index = packet->stream_index > video_stream_index ? packet->stream_index : packet->stream_index + 1;
            packet->stream_index = out_index;
            av_packet_rescale_ts(packet, input_ctx->streams[packet->stream_index]->time_base, output_ctx->streams[out_index]->time_base);
            ret = av_interleaved_write_frame(output_ctx, packet);
            if (ret < 0) {
                fprintf(stderr, "Error writing packet\n");
                break;
            }
        }
        av_packet_unref(packet);
    }
    avcodec_send_frame(enc_ctx, NULL);
    while (1) {
        AVPacket *enc_packet = av_packet_alloc();
        ret = avcodec_receive_packet(enc_ctx, enc_packet);
        if (ret < 0) break;
        enc_packet->stream_index = 0;
        av_packet_rescale_ts(enc_packet, enc_ctx->time_base, out_stream->time_base);
        av_interleaved_write_frame(output_ctx, enc_packet);
        av_packet_free(&enc_packet);
    }
    finish_progress();
    printf("Conversion complete!\n");
    av_write_trailer(output_ctx);
    av_packet_free(&packet);
    av_frame_free(&frame);
    avcodec_free_context(&enc_ctx);
    avcodec_free_context(&dec_ctx);
    avformat_close_input(&input_ctx);
    if (output_ctx && !(output_ctx->oformat->flags & AVFMT_NOFILE)) {
        avio_closep(&output_ctx->pb);
    }
    avformat_free_context(output_ctx);
    av_buffer_unref(&hw_device_ctx);
    return 0;
}

int extract_audio(const char *input, const char *output, const char *acodec) {
    AVFormatContext *input_ctx = NULL;
    AVFormatContext *output_ctx = NULL;
    int ret;
    ret = avformat_open_input(&input_ctx, input, NULL, NULL);
    if (ret < 0) return ret;
    ret = avformat_find_stream_info(input_ctx, NULL);
    if (ret < 0) return ret;
    ret = avformat_alloc_output_context2(&output_ctx, NULL, NULL, output);
    if (ret < 0) return ret;
    int audio_stream_index = -1;
    for (int i = 0; i < input_ctx->nb_streams; i++) {
        if (input_ctx->streams[i]->codecpar->codec_type == AVMEDIA_TYPE_AUDIO) {
            audio_stream_index = i;
            AVStream *in_stream = input_ctx->streams[i];
            AVStream *out_stream = avformat_new_stream(output_ctx, NULL);
            ret = avcodec_parameters_copy(out_stream->codecpar, in_stream->codecpar);
            if (ret < 0) return ret;
            out_stream->codecpar->codec_tag = 0;
            break;
        }
    }
    if (audio_stream_index == -1) {
        fprintf(stderr, "No audio stream found\n");
        return -1;
    }
    if (!(output_ctx->oformat->flags & AVFMT_NOFILE)) {
        ret = avio_open(&output_ctx->pb, output, AVIO_FLAG_WRITE);
        if (ret < 0) return ret;
    }
    ret = avformat_write_header(output_ctx, NULL);
    if (ret < 0) return ret;
    AVPacket packet;
    while (1) {
        ret = av_read_frame(input_ctx, &packet);
        if (ret < 0) break;
        if (packet.stream_index == audio_stream_index) {
            packet.stream_index = 0;
            av_packet_rescale_ts(&packet, input_ctx->streams[audio_stream_index]->time_base, output_ctx->streams[0]->time_base);
            ret = av_interleaved_write_frame(output_ctx, &packet);
            if (ret < 0) break;
        }
        av_packet_unref(&packet);
    }
    av_write_trailer(output_ctx);
    avformat_close_input(&input_ctx);
    avio_closep(&output_ctx->pb);
    avformat_free_context(output_ctx);
    return 0;
}

int cut_video(const char *input, const char *output, double start, double duration) {
    // Similar to convert, but skip frames outside time range
    // For simplicity, use convert and add time filter, but here basic skip
    AVFormatContext *input_ctx = NULL;
    AVFormatContext *output_ctx = NULL;
    AVCodec *decoder = NULL;
    AVCodec *encoder = NULL;
    AVCodecContext *dec_ctx = NULL;
    AVCodecContext *enc_ctx = NULL;
    AVFrame *frame = NULL;
    AVPacket *packet = NULL;
    int ret;
    ret = avformat_open_input(&input_ctx, input, NULL, NULL);
    if (ret < 0) return ret;
    ret = avformat_find_stream_info(input_ctx, NULL);
    if (ret < 0) return ret;
    int video_stream_index = -1;
    for (int i = 0; i < input_ctx->nb_streams; i++) {
        if (input_ctx->streams[i]->codecpar->codec_type == AVMEDIA_TYPE_VIDEO) {
            video_stream_index = i;
            break;
        }
    }
    if (video_stream_index == -1) return -1;
    decoder = avcodec_find_decoder(input_ctx->streams[video_stream_index]->codecpar->codec_id);
    dec_ctx = avcodec_alloc_context3(decoder);
    avcodec_parameters_to_context(dec_ctx, input_ctx->streams[video_stream_index]->codecpar);
    avcodec_open2(dec_ctx, decoder, NULL);
    encoder = avcodec_find_encoder(dec_ctx->codec_id);
    enc_ctx = avcodec_alloc_context3(encoder);
    enc_ctx->width = dec_ctx->width;
    enc_ctx->height = dec_ctx->height;
    enc_ctx->pix_fmt = dec_ctx->pix_fmt;
    enc_ctx->time_base = dec_ctx->time_base;
    avcodec_open2(enc_ctx, encoder, NULL);
    ret = avformat_alloc_output_context2(&output_ctx, NULL, NULL, output);
    AVStream *out_stream = avformat_new_stream(output_ctx, NULL);
    avcodec_parameters_from_context(out_stream->codecpar, enc_ctx);
    avio_open(&output_ctx->pb, output, AVIO_FLAG_WRITE);
    avformat_write_header(output_ctx, NULL);
    frame = av_frame_alloc();
    packet = av_packet_alloc();
    double end_time = start + duration;

    // Initialize progress bar for cut operation
    ProgressContext progress;
    init_progress(&progress, (int64_t)(duration * AV_TIME_BASE));

    while (1) {
        ret = av_read_frame(input_ctx, packet);
        if (ret < 0) break;
        if (packet->stream_index == video_stream_index) {
            double pts_time = av_q2d(input_ctx->streams[video_stream_index]->time_base) * packet->pts;
            if (pts_time >= start && pts_time <= end_time) {
                // Update progress
                double current_progress_time = pts_time - start;
                update_progress(&progress, (int64_t)(current_progress_time / av_q2d(input_ctx->streams[video_stream_index]->time_base)),
                               input_ctx->streams[video_stream_index]->time_base);

                ret = avcodec_send_packet(dec_ctx, packet);
                while (ret >= 0) {
                    ret = avcodec_receive_frame(dec_ctx, frame);
                    if (ret < 0) break;
                    ret = avcodec_send_frame(enc_ctx, frame);
                    while (ret >= 0) {
                        AVPacket *enc_packet = av_packet_alloc();
                        ret = avcodec_receive_packet(enc_ctx, enc_packet);
                        if (ret < 0) break;
                        enc_packet->stream_index = 0;
                        av_packet_rescale_ts(enc_packet, enc_ctx->time_base, out_stream->time_base);
                        av_interleaved_write_frame(output_ctx, enc_packet);
                        av_packet_free(&enc_packet);
                    }
                }
            }
        }
        av_packet_unref(packet);
    }
    avcodec_send_frame(enc_ctx, NULL);
    while (1) {
        AVPacket *enc_packet = av_packet_alloc();
        ret = avcodec_receive_packet(enc_ctx, enc_packet);
        if (ret < 0) break;
        enc_packet->stream_index = 0;
        av_packet_rescale_ts(enc_packet, enc_ctx->time_base, out_stream->time_base);
        av_interleaved_write_frame(output_ctx, enc_packet);
        av_packet_free(&enc_packet);
    }
    finish_progress();
    printf("Cut complete!\n");
    av_write_trailer(output_ctx);
    // cleanup similar
    return 0;
}

int merge_videos(const char *output, int num_inputs, char *inputs[]) {
    // Use concat demuxer
    char concat_file[1024] = "ffconcat version 1.0\n";
    for (int i = 0; i < num_inputs; i++) {
        strcat(concat_file, "file '");
        strcat(concat_file, inputs[i]);
        strcat(concat_file, "'\n");
    }
    FILE *f = fopen("concat.txt", "w");
    fprintf(f, "%s", concat_file);
    fclose(f);
    // Then transcode concat.txt to output
    // For simplicity, assume same format, copy
    AVFormatContext *input_ctx = NULL;
    AVFormatContext *output_ctx = NULL;
    int ret = avformat_open_input(&input_ctx, "concat.txt", av_find_input_format("concat"), NULL);
    if (ret < 0) return ret;
    ret = avformat_find_stream_info(input_ctx, NULL);
    ret = avformat_alloc_output_context2(&output_ctx, NULL, NULL, output);
    for (int i = 0; i < input_ctx->nb_streams; i++) {
        AVStream *in_stream = input_ctx->streams[i];
        AVStream *out_stream = avformat_new_stream(output_ctx, NULL);
        avcodec_parameters_copy(out_stream->codecpar, in_stream->codecpar);
    }
    avio_open(&output_ctx->pb, output, AVIO_FLAG_WRITE);
    avformat_write_header(output_ctx, NULL);
    AVPacket packet;
    while (1) {
        ret = av_read_frame(input_ctx, &packet);
        if (ret < 0) break;
        av_packet_rescale_ts(&packet, input_ctx->streams[packet.stream_index]->time_base, output_ctx->streams[packet.stream_index]->time_base);
        av_interleaved_write_frame(output_ctx, &packet);
        av_packet_unref(&packet);
    }
    av_write_trailer(output_ctx);
    // cleanup
    return 0;
}

int add_filter(const char *input, const char *output, const char *filter_str) {
    // Use libavfilter for filters like crop, blur, etc.
    // This is complex, but basic setup
    // For now, stub
    printf("Filter not implemented yet\n");
    return -1;
}

int show_info(const char *input) {
    AVFormatContext *input_ctx = NULL;
    int ret = avformat_open_input(&input_ctx, input, NULL, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot open input file\n");
        return ret;
    }
    ret = avformat_find_stream_info(input_ctx, NULL);
    if (ret < 0) {
        fprintf(stderr, "Cannot find stream info\n");
        avformat_close_input(&input_ctx);
        return ret;
    }
    av_dump_format(input_ctx, 0, input, 0);
    avformat_close_input(&input_ctx);
    return 0;
}

int main(int argc, char *argv[]) {
    av_log_set_level(AV_LOG_INFO);
    if (argc < 2) {
        print_usage();
        return 1;
    }
    char *operation = argv[1];
    if (strcmp(operation, "info") == 0) {
        if (argc < 3) {
            printf("Usage: info <input>\n");
            return 1;
        }
        return show_info(argv[2]);
    } else if (strcmp(operation, "convert") == 0) {
        if (argc < 4) {
            printf("Usage: convert <input> <output> [vcodec] [bitrate] [resolution] [fps]\n");
            return 1;
        }
        char *input = argv[2];
        char *output = argv[3];
        char *vcodec = argc > 4 ? argv[4] : "h264";
        char *bitrate = argc > 5 ? argv[5] : NULL;
        char *resolution = argc > 6 ? argv[6] : NULL;
        char *fps = argc > 7 ? argv[7] : NULL;
        return convert_video(input, output, vcodec, bitrate, resolution, fps);
    } else if (strcmp(operation, "extract_audio") == 0) {
        if (argc < 4) {
            printf("Usage: extract_audio <input> <output> [acodec]\n");
            return 1;
        }
        char *input = argv[2];
        char *output = argv[3];
        char *acodec = argc > 4 ? argv[4] : NULL;
        return extract_audio(input, output, acodec);
    } else if (strcmp(operation, "cut") == 0) {
        if (argc < 6) {
            printf("Usage: cut <input> <output> <start> <duration>\n");
            return 1;
        }
        char *input = argv[2];
        char *output = argv[3];
        double start = atof(argv[4]);
        double duration = atof(argv[5]);
        return cut_video(input, output, start, duration);
    } else if (strcmp(operation, "merge") == 0) {
        if (argc < 4) {
            printf("Usage: merge <output> <input1> <input2> ...\n");
            return 1;
        }
        char *output = argv[2];
        return merge_videos(output, argc - 3, &argv[3]);
    } else if (strcmp(operation, "add_filter") == 0) {
        if (argc < 5) {
            printf("Usage: add_filter <input> <output> <filter>\n");
            return 1;
        }
        char *input = argv[2];
        char *output = argv[3];
        char *filter = argv[4];
        return add_filter(input, output, filter);
    } else {
        printf("Unknown operation\n");
        print_usage();
        return 1;
    }
    printf("Operation completed successfully\n");
    return 0;
}
