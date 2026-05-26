package mlmodel

import (
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/DungNguyen0209/aibodyguard/internal/modelcache"
)

// Model wraps an ONNX Runtime session for the distilbert-secret-masker model.
type Model struct {
	session *ort.DynamicAdvancedSession
}

// New loads the ONNX Runtime shared library and the distilbert model from cacheDir.
// Returns an error if either is missing or fails to load.
func New(cacheDir string) (*Model, error) {
	libPath := modelcache.LibPath(cacheDir)
	modelPath := modelcache.ModelPath(cacheDir)

	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"logits"},
		nil,
	)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("load model: %w", err)
	}

	return &Model{session: session}, nil
}

// Available returns true if the model is loaded and ready for inference.
func (m *Model) Available() bool {
	return m != nil && m.session != nil
}

// Predict runs inference on a single chunk of token IDs.
// Returns a float32 slice of SECRET probability scores, one per input token.
// Scores are derived from the logits via softmax over the label classes.
func (m *Model) Predict(tokenIDs []int64, attentionMask []int64) ([]float32, error) {
	if !m.Available() {
		return nil, fmt.Errorf("model not available")
	}

	seqLen := int64(len(tokenIDs))
	shape := ort.NewShape(1, seqLen)

	inputIDsTensor, err := ort.NewTensor(shape, tokenIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	maskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	// Run inference — pre-allocate output tensor for logits [1, seqLen, numLabels=3]
	numLabels := int64(3) // O, B-SECRET, I-SECRET
	logitsShape := ort.NewShape(1, seqLen, numLabels)
	logitsTensor, err := ort.NewEmptyTensor[float32](logitsShape)
	if err != nil {
		return nil, fmt.Errorf("create logits tensor: %w", err)
	}
	defer logitsTensor.Destroy()

	if err := m.session.Run(
		[]ort.Value{inputIDsTensor, maskTensor},
		[]ort.Value{logitsTensor},
	); err != nil {
		return nil, fmt.Errorf("inference: %w", err)
	}

	rawLogits := logitsTensor.GetData()
	scores := make([]float32, seqLen)
	for i := range tokenIDs {
		base := i * int(numLabels)
		// Softmax over [O, B-SECRET, I-SECRET]; combine B+I as SECRET probability
		maxL := math.Max(float64(rawLogits[base]), math.Max(float64(rawLogits[base+1]), float64(rawLogits[base+2])))
		expO := math.Exp(float64(rawLogits[base]) - maxL)
		expB := math.Exp(float64(rawLogits[base+1]) - maxL)
		expI := math.Exp(float64(rawLogits[base+2]) - maxL)
		sumExp := expO + expB + expI
		scores[i] = float32((expB + expI) / sumExp)
	}
	return scores, nil
}

// Close releases ONNX Runtime resources.
func (m *Model) Close() {
	if m == nil {
		return
	}
	if m.session != nil {
		m.session.Destroy()
		m.session = nil
	}
	ort.DestroyEnvironment()
}
