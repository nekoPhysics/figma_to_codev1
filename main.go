package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// LayerInfo は抽出したいレイヤー情報を表します
type LayerInfo struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Type                string               `json:"type"`
	ParentID            string               `json:"parent_id,omitempty"`
	AbsoluteBoundingBox *AbsoluteBoundingBox `json:"absoluteBoundingBox,omitempty"`
	Styles              map[string]string    `json:"styles,omitempty"`
	Constraints         *Constraints         `json:"constraints,omitempty"`
}

// AbsoluteBoundingBox はノードの絶対的なバウンディングボックスを表します
type AbsoluteBoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Constraints はノードの制約を表します
type Constraints struct {
	Vertical   string `json:"vertical"`
	Horizontal string `json:"horizontal"`
}

// FigmaFile APIレスポンスをマッピングする構造体
type FigmaFile struct {
	Name       string                   `json:"name"`
	Document   FigmaDocument            `json:"document"`
	Components map[string]FigmaComponent `json:"components"`
}

type FigmaDocument struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children []FigmaNode `json:"children"`
}

type FigmaNode struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Type                string               `json:"type"`
	AbsoluteBoundingBox *AbsoluteBoundingBox `json:"absoluteBoundingBox,omitempty"`
	Styles              map[string]string    `json:"styles,omitempty"`
	Constraints         *Constraints         `json:"constraints,omitempty"`
	Children            []FigmaNode          `json:"children,omitempty"`
	// 必要に応じて他のフィールドを追加してください
}

type FigmaComponent struct {
	// 必要に応じてFigma APIのレスポンスに基づいてフィールドを定義してください
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("使用方法: go run main.go <Figma_URL> <ページ名>")
		return
	}

	figmaURL := os.Args[1]
	pageName := os.Args[2]

	// 環境変数からAPIトークンを取得
	figmaAPIToken := os.Getenv("FIGMA_API_TOKEN")
	if figmaAPIToken == "" {
		fmt.Println("FIGMA_API_TOKEN 環境変数が設定されていません")
		return
	}

	// 提供されたURLからファイルIDを抽出
	fileID, err := extractFileID(figmaURL)
	if err != nil {
		fmt.Printf("ファイルIDの抽出に失敗しました: %v\n", err)
		return
	}

	// HTTPクライアントの作成
	client := &http.Client{}

	// Figma APIへのGETリクエストを作成
	apiURL := fmt.Sprintf("https://api.figma.com/v1/files/%s", fileID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		fmt.Printf("HTTPリクエストの作成に失敗しました: %v\n", err)
		return
	}

	// 認証ヘッダーにFigma APIトークンを設定
	req.Header.Set("X-Figma-Token", figmaAPIToken)

	// リクエストを実行
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("HTTPリクエストの実行に失敗しました: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// レスポンスのステータスを確認
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("HTTPエラー: %s\nレスポンスボディ: %s\n", resp.Status, string(bodyBytes))
		return
	}

	// JSONレスポンスをデコード
	var figmaFileData FigmaFile
	if err := json.NewDecoder(resp.Body).Decode(&figmaFileData); err != nil {
		fmt.Printf("JSONレスポンスのデコードに失敗しました: %v\n", err)
		return
	}

	// 指定されたページを検索
	var targetPage *FigmaNode
	for _, child := range figmaFileData.Document.Children {
		if child.Name == pageName && child.Type == "CANVAS" {
			targetPage = &child
			break
		}
	}

	if targetPage == nil {
		fmt.Printf("指定されたページ '%s' が見つかりませんでした。\n", pageName)
		return
	}

	// レイヤー情報を抽出
	var layers []LayerInfo
	extractLayers(*targetPage, "", &layers)

	// レイヤー情報をJSONにシリアライズ
	jsonData, err := json.MarshalIndent(layers, "", "  ")
	if err != nil {
		fmt.Printf("レイヤー情報のJSONシリアライズに失敗しました: %v\n", err)
		return
	}

	// JSONをファイルに出力
	outputFile := "layers.json"
	if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
		fmt.Printf("JSONファイルへの書き込みに失敗しました: %v\n", err)
		return
	}

	fmt.Printf("レイヤー情報が %s に正常に書き込まれました。\n", outputFile)
}

// extractFileID はFigmaのURLからファイルIDを抽出します。
// 'file/'および'design/'のパスに対応しています。
func extractFileID(figmaURL string) (string, error) {
	parsedURL, err := url.Parse(figmaURL)
	if err != nil {
		return "", fmt.Errorf("URLの解析に失敗しました: %w", err)
	}

	// パスを分割
	pathSegments := strings.Split(parsedURL.Path, "/")
	if len(pathSegments) < 3 {
		return "", fmt.Errorf("URLパスに十分なセグメントが含まれていません")
	}

	prefix := pathSegments[1]
	id := pathSegments[2]

	if prefix != "file" && prefix != "design" {
		return "", fmt.Errorf("URLパスは '/file/' または '/design/' で始まる必要があります")
	}

	// IDの形式を簡単に検証（FigmaのIDは一般的に英数字）
	matched, err := regexp.MatchString(`^[A-Za-z0-9]+$`, id)
	if err != nil {
		return "", fmt.Errorf("ID形式の検証中にエラーが発生しました: %w", err)
	}
	if !matched {
		return "", fmt.Errorf("URLから抽出したIDの形式が無効です")
	}

	return id, nil
}

// extractLayers はFigmaのドキュメントツリーを再帰的に走査してレイヤー情報を抽出します
func extractLayers(node FigmaNode, parentID string, layers *[]LayerInfo) {
	layer := LayerInfo{
		ID:       node.ID,
		Name:     node.Name,
		Type:     node.Type,
		ParentID: parentID,
	}

	// AbsoluteBoundingBox が存在する場合は追加
	if node.AbsoluteBoundingBox != nil {
		layer.AbsoluteBoundingBox = node.AbsoluteBoundingBox
	}

	// Styles が存在する場合は追加
	if node.Styles != nil {
		layer.Styles = node.Styles
	}

	// Constraints が存在する場合は追加
	if node.Constraints != nil {
		layer.Constraints = node.Constraints
	}

	*layers = append(*layers, layer)

	for _, child := range node.Children {
		extractLayers(child, node.ID, layers)
	}
}