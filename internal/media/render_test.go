package media

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"volnorez/internal/tools"
)

type renderRunner struct {
	command tools.Command
	result  tools.Result
	err     error
}

func (r *renderRunner) LookPath(string) (string, error) { return "", errors.New("not implemented") }

func (r *renderRunner) Run(_ context.Context, command tools.Command) (tools.Result, error) {
	r.command = command
	return r.result, r.err
}

func renderRequest() RenderRequest {
	return RenderRequest{
		FFmpeg:  "/bin/ffmpeg",
		Input:   "/in.mp3",
		Cover:   "/work/cover.png",
		ASS:     "/work/subtitles.ass",
		FontDir: "/work/fonts",
		WorkDir: "/work",
		Output:  "/out.tmp.mp4",
		Accent:  "#FFD84D",
		Selection: Selection{
			Start:    80 * time.Second,
			Duration: 45 * time.Second,
		},
	}
}

func TestBuildRenderCommandContainsOutputContract(t *testing.T) {
	cmd := BuildRenderCommand(renderRequest())
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"showwaves",
		"geq",
		"subtitles=subtitles.ass:fontsdir=fonts",
		"libx264",
		"yuv420p",
		"aac",
		"192k",
		"+faststart",
		"1080x1920",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if cmd.Name != "/bin/ffmpeg" {
		t.Fatalf("name = %q", cmd.Name)
	}
	if cmd.Dir != "/work" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func TestBuildRenderCommandUsesRequiredFilterGraph(t *testing.T) {
	cmd := BuildRenderCommand(renderRequest())
	filter := argumentAfter(t, cmd.Args, "-filter_complex")
	for _, want := range []string{
		"color=c=0x101014:s=1080x1920:r=30:d=45.000[bg]",
		"[1:v]scale=360:360,format=rgba,geq=",
		"hypot(X-W/2\\,Y-H/2)",
		"[0:a]atrim=start=80.000:duration=45.000,asetpts=PTS-STARTPTS,asplit=2[aout][aw]",
		"[aw]showwaves=s=900x420:mode=line:rate=30:colors=0xFFD84D,format=rgba[wave]",
		"[bg][cover]overlay=x=(W-w)/2:y=240:eof_action=repeat[tmp1]",
		"[tmp1][wave]overlay=x=(W-w)/2:y=760:shortest=1[tmp2]",
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter graph missing %q in %s", want, filter)
		}
	}
}

func TestBuildRenderCommandKeepsPathsAsArguments(t *testing.T) {
	req := renderRequest()
	req.Input = "/audio/input with spaces.mp3"
	req.Cover = "/work/cover with spaces.png"
	req.Output = "/output/final with spaces.mp4"
	cmd := BuildRenderCommand(req)
	for _, want := range []string{req.Input, req.Cover, req.Output} {
		if !containsArgument(cmd.Args, want) {
			t.Errorf("missing argument %q in %#v", want, cmd.Args)
		}
	}
}

func TestRenderRunsBuiltCommand(t *testing.T) {
	r := &renderRunner{}
	req := renderRequest()
	if err := Render(context.Background(), r, req); err != nil {
		t.Fatal(err)
	}
	if want := BuildRenderCommand(req); !reflect.DeepEqual(r.command, want) {
		t.Fatalf("command = %#v, want %#v", r.command, want)
	}
}

func TestRenderWrapsRunnerFailure(t *testing.T) {
	r := &renderRunner{err: errors.New("exit status 1")}
	err := Render(context.Background(), r, renderRequest())
	if err == nil || !strings.Contains(err.Error(), "render") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyOutputAcceptsValidFile(t *testing.T) {
	r := &renderRunner{result: tools.Result{Stdout: []byte(validRenderProbeJSON)}}
	output := writeTestMP4(t, true)
	if err := VerifyOutput(context.Background(), r, "/bin/ffprobe", output, 45*time.Second); err != nil {
		t.Fatal(err)
	}
	want := tools.Command{
		Name: "/bin/ffprobe",
		Args: []string{
			"-v", "error",
			"-show_entries", "format=format_name,duration:stream=codec_type,codec_name,width,height,pix_fmt,avg_frame_rate,channels,bit_rate",
			"-of", "json",
			output,
		},
	}
	if !reflect.DeepEqual(r.command, want) {
		t.Fatalf("command = %#v, want %#v", r.command, want)
	}
}

func TestVerifyOutputRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "missing audio", output: `{"format":{"duration":"45.000"},"streams":[{"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"pix_fmt":"yuv420p","avg_frame_rate":"30/1"}]}`},
		{name: "missing video", output: `{"format":{"duration":"45.000"},"streams":[{"codec_type":"audio","codec_name":"aac"}]}`},
		{name: "wrong dimensions", output: replaceProbeField(validRenderProbeJSON, `"width":1080`, `"width":1920`)},
		{name: "wrong frame rate", output: replaceProbeField(validRenderProbeJSON, `"avg_frame_rate":"30/1"`, `"avg_frame_rate":"30000/1001"`)},
		{name: "wrong video codec", output: replaceProbeField(validRenderProbeJSON, `"codec_name":"h264"`, `"codec_name":"hevc"`)},
		{name: "wrong audio codec", output: replaceProbeField(validRenderProbeJSON, `"codec_name":"aac"`, `"codec_name":"mp3"`)},
		{name: "wrong container", output: replaceProbeField(validRenderProbeJSON, `"format_name":"mov,mp4,m4a,3gp,3g2,mj2"`, `"format_name":"matroska,webm"`)},
		{name: "mono audio", output: replaceProbeField(validRenderProbeJSON, `"channels":2`, `"channels":1`)},
		{name: "audio bitrate too low", output: replaceProbeField(validRenderProbeJSON, `"bit_rate":"192000"`, `"bit_rate":"64000"`)},
		{name: "audio bitrate too high", output: replaceProbeField(validRenderProbeJSON, `"bit_rate":"192000"`, `"bit_rate":"250000"`)},
		{name: "wrong pixel format", output: replaceProbeField(validRenderProbeJSON, `"pix_fmt":"yuv420p"`, `"pix_fmt":"yuv444p"`)},
		{name: "duration delta over limit", output: replaceProbeField(validRenderProbeJSON, `"duration":"45.000"`, `"duration":"45.201"`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &renderRunner{result: tools.Result{Stdout: []byte(test.output)}}
			err := VerifyOutput(context.Background(), r, "/bin/ffprobe", writeTestMP4(t, true), 45*time.Second)
			if err == nil {
				t.Fatal("VerifyOutput returned nil error")
			}
		})
	}
}

func TestVerifyOutputAcceptsDurationDeltaAtLimit(t *testing.T) {
	output := replaceProbeField(validRenderProbeJSON, `"duration":"45.000"`, `"duration":"45.200"`)
	r := &renderRunner{result: tools.Result{Stdout: []byte(output)}}
	if err := VerifyOutput(context.Background(), r, "/bin/ffprobe", writeTestMP4(t, true), 45*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyOutputAllowsShortFileBitrateVariance(t *testing.T) {
	output := replaceProbeField(validRenderProbeJSON, `"duration":"45.000"`, `"duration":"1.000"`)
	output = replaceProbeField(output, `"bit_rate":"192000"`, `"bit_rate":"100000"`)
	r := &renderRunner{result: tools.Result{Stdout: []byte(output)}}
	if err := VerifyOutput(context.Background(), r, "/bin/ffprobe", writeTestMP4(t, true), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyOutputAllowsContentDependentAverageBitrate(t *testing.T) {
	output := replaceProbeField(validRenderProbeJSON, `"bit_rate":"192000"`, `"bit_rate":"81389"`)
	r := &renderRunner{result: tools.Result{Stdout: []byte(output)}}
	if err := VerifyOutput(context.Background(), r, "/bin/ffprobe", writeTestMP4(t, true), 45*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyOutputRejectsNonFaststartMP4(t *testing.T) {
	r := &renderRunner{result: tools.Result{Stdout: []byte(validRenderProbeJSON)}}
	err := VerifyOutput(context.Background(), r, "/bin/ffprobe", writeTestMP4(t, false), 45*time.Second)
	if err == nil || !strings.Contains(err.Error(), "faststart") {
		t.Fatalf("error = %v", err)
	}
}

const validRenderProbeJSON = `{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"45.000"},"streams":[{"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"pix_fmt":"yuv420p","avg_frame_rate":"30/1"},{"codec_type":"audio","codec_name":"aac","channels":2,"bit_rate":"192000"}]}`

func writeTestMP4(t *testing.T, faststart bool) string {
	t.Helper()
	box := func(name string, payload []byte) []byte {
		data := make([]byte, 8+len(payload))
		binary.BigEndian.PutUint32(data, uint32(len(data)))
		copy(data[4:8], name)
		copy(data[8:], payload)
		return data
	}
	boxes := [][]byte{box("ftyp", []byte("isom\x00\x00\x00\x00isom"))}
	if faststart {
		boxes = append(boxes, box("moov", nil), box("mdat", []byte{0}))
	} else {
		boxes = append(boxes, box("mdat", []byte{0}), box("moov", nil))
	}
	var data []byte
	for _, current := range boxes {
		data = append(data, current...)
	}
	path := filepath.Join(t.TempDir(), "output.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func argumentAfter(t *testing.T, args []string, option string) string {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == option {
			return args[i+1]
		}
	}
	t.Fatalf("missing option %q in %#v", option, args)
	return ""
}

func containsArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func replaceProbeField(output, old, replacement string) string {
	return strings.Replace(output, old, replacement, 1)
}
