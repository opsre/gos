package httpapi

// GenericResponse 表示无法用固定结构描述的通用成功响应。
type GenericResponse struct {
	Data map[string]interface{} `json:"data"`
}
