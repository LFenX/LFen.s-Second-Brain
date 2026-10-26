// Code generated from the fixed synthetic fixture manifest; DO NOT EDIT.
package structure

type fixtureFact struct {
	ID, Path, Kind, Anchor, SHA256 string
	Start, End, Line               int
}

var fixtureInputs = []Input{
	{SourceID: "00000000000000000000000000000001", RevisionID: "00000000000000000000000000000065", Path: "frontend/data-table.tsx", Parser: "tsx", Content: []byte("\ufeff// 中文😀：这里的 import ghost from \"not-real\" 是注释\r\nimport type { Row } from \"@/types\";\r\nimport { normalize } from \"./utils\";\r\nexport function DataTable(rows: Row[]) {\r\n  const label = normalize(rows[0]?.label ?? \"\");\r\n  return <section><span>{label}</span><b>{rows.length}</b></section>;\r\n}\r\nexport function identity<Row>(value: Row): Row { return value; }\r\nconst fakeImport = \"import missing from not-a-module\";\r\nexport function dynamicCall(fn: () => void) { fn(); }\r\n")},
	{SourceID: "00000000000000000000000000000002", RevisionID: "00000000000000000000000000000066", Path: "frontend/types.ts", Parser: "typescript", Content: []byte("export type RowId = string;\nexport interface Row { id: RowId; label: string; }\nexport interface Column<T> { key: keyof T; name: string; }\nexport type Rows = readonly Row[];\n")},
	{SourceID: "00000000000000000000000000000003", RevisionID: "00000000000000000000000000000067", Path: "frontend/utils.ts", Parser: "typescript", Content: []byte("export function normalize(text: string): string {\n  return text.trim().toLowerCase();\n}\nexport const lower = normalize;\nexport async function loadPart(modulePath: string) {\n  return import(modulePath);\n}\nexport function duplicated() { return \"same\"; }\nexport function otherDuplicated() { return \"same\"; }\nconst regularText = \"export function Ghost() {}\";\n")},
	{SourceID: "00000000000000000000000000000004", RevisionID: "00000000000000000000000000000068", Path: "frontend/index.ts", Parser: "typescript", Content: []byte("export { DataTable } from \"./data-table\";\nexport type { Row, Column } from \"./types\";\nexport * from \"./types\";\n")},
	{SourceID: "00000000000000000000000000000005", RevisionID: "00000000000000000000000000000069", Path: "backend/table.go", Parser: "go", Content: []byte("\ufeffpackage fixture\r\nimport \"strings\"\r\n//line imaginary.go:900\r\ntype Row struct { ID string; Label string }\r\nfunc (r Row) Display() string { return r.Label }\r\nfunc clean(s string) string { return strings.TrimSpace(s) }\r\nfunc Normalize(s string) string { return clean(s) }\r\nfunc NewRow(id string) Row { return Row{ID: id} }\r\nfunc CrossCall(s string) string { return CrossFile(s) }\r\nvar Fake = \"import ghost\"\r\n")},
	{SourceID: "00000000000000000000000000000006", RevisionID: "0000000000000000000000000000006a", Path: "backend/types.go", Parser: "go", Content: []byte("package fixture\ntype RemoteRow struct { Value string }\nfunc CrossFile(s string) string { return s }\nfunc LocalUse(r RemoteRow) string { return r.Value }\n")},
	{SourceID: "00000000000000000000000000000007", RevisionID: "0000000000000000000000000000006b", Path: "README.md", Parser: "markdown", Content: []byte("# 合成项目\n\n一句话也保留为完整资料。\n\n## 前端\nDataTable 只是本批合成组件，不含真实 AICS 代码。\n\n## 后端\nNormalize 与 Row 是本批合成代码。\n\n![仅保留引用](https://invalid.example/never-download.png)\n")},
	{SourceID: "00000000000000000000000000000008", RevisionID: "0000000000000000000000000000006c", Path: "docs/guide.md", Parser: "markdown", Content: []byte("# 阅读指南\r\n\r\n## 重复标题\r\n第一处正文😀。\r\n\r\n```tsx\r\n# 围栏中的伪标题\r\nconst sample = \"import fake from nowhere\";\r\n```\r\n\r\n## 重复标题\r\n第二处正文。\r\n\r\nSetext 章节\r\n-----------\r\n\r\n| 字段 | 说明 |\r\n| --- | --- |\r\n| a | 只保留原文，首版不伪造表格节点 |\r\n\r\n<img src=\"https://invalid.example/not-a-fetch.png\" onerror=\"invalid()\">\r\n")},
}

var fixtureSHA = map[string]string{
	"frontend/data-table.tsx": "631021edb94a6c8134f8cbf0be10f987e7788ca0f8739695cbe2fbda60f12b01",
	"frontend/types.ts":       "1500576f0f3ff67d73278fbaf71746a71ff1d4c54e0ecd6ceec563f9b819820d",
	"frontend/utils.ts":       "231cbd121c8bec1f2bc1e3eeedbc192dfdaf106892033ed0133c789238a1ce25",
	"frontend/index.ts":       "8e0b50723c6ce21df12cc6aab83996a6aa351f4b0624235b57de652a54ffc1db",
	"backend/table.go":        "b335fc31ba353f52aae7592ebc4cd84cc9232f0399e5d45451dd87b307a6c2d6",
	"backend/types.go":        "a2f86d6086608dc4be8e512093b20ec200ab8cc194ed993ebe9a65d11e488904",
	"README.md":               "80aaaace831049afe68ccb47aa780df33a428a1ddeeee3fd8a3fcd1a505935e9",
	"docs/guide.md":           "bb9ec58ccfb3f5a67ba0e52c1f42eee3dbba43ccf65f0935e3dd81585fd28d0c",
}

var fixtureFacts = []fixtureFact{
	{ID: "F001", Path: "frontend/data-table.tsx", Kind: "symbol", Anchor: "function DataTable(rows: Row[])", SHA256: "73585271a953ef141044bae1e91467930a55ca04a085aecc6dda9d6fd63c6590", Start: 151, End: 182, Line: 4},
	{ID: "F002", Path: "frontend/data-table.tsx", Kind: "resolved_import", Anchor: "@/types", SHA256: "6a7b27477fdb53f28e1ad6807f59f4693469a33669088cf42f16b29077e63b02", Start: 95, End: 102, Line: 2},
	{ID: "F003", Path: "frontend/data-table.tsx", Kind: "resolved_import", Anchor: "./utils", SHA256: "760ffb92a3ead9b2961b358ce267a6666171c2ac7765926349c904a0bac5cfe6", Start: 133, End: 140, Line: 3},
	{ID: "F004", Path: "frontend/data-table.tsx", Kind: "resolved_type", Anchor: "rows: Row[]", SHA256: "21b6b3854b71765f03d4ea5a2e0f3fb0403e59611d2f29f3db0c4cf090d09e33", Start: 170, End: 181, Line: 4},
	{ID: "F005", Path: "frontend/data-table.tsx", Kind: "resolved_call", Anchor: "normalize(rows[0]", SHA256: "445fc859f9ed73c984c6bc195b51b6717cb62ae5931b2f2e9db9f23c31672da7", Start: 202, End: 219, Line: 5},
	{ID: "F006", Path: "frontend/data-table.tsx", Kind: "symbol", Anchor: "function identity<Row>", SHA256: "8e8715d19f3a51f9b6c94d99f1f124b8f7f821f738ca08e82c11e0a03f79609a", Start: 317, End: 339, Line: 8},
	{ID: "F007", Path: "frontend/data-table.tsx", Kind: "no_false_type", Anchor: "value: Row", SHA256: "b946f64c3f4b4a00ac287c44d8183e171ce7063401357e59133471b82bfed3a9", Start: 340, End: 350, Line: 8},
	{ID: "F008", Path: "frontend/data-table.tsx", Kind: "no_false_import", Anchor: "import ghost from \"not-real\"", SHA256: "b4d46d7adfb2d18a236e2177027c9fc7a768fbfc22d0be90528663428e6350ed", Start: 29, End: 57, Line: 1},
	{ID: "F009", Path: "frontend/data-table.tsx", Kind: "no_false_import", Anchor: "import missing from not-a-module", SHA256: "9a6c434a407e26960cd1bb9a06d052b628fa220c6adb4c09a5d7ba41f1fb8e5b", Start: 396, End: 428, Line: 9},
	{ID: "F010", Path: "frontend/data-table.tsx", Kind: "unresolved_call", Anchor: "fn();", SHA256: "27b91746b06d3eabf8246d97bfa8af4ca30b5776d4c51f0968872b55c5e77dab", Start: 478, End: 483, Line: 10},
	{ID: "F011", Path: "frontend/data-table.tsx", Kind: "raw_coordinate", Anchor: "中文😀", SHA256: "e973a1c1b41c5c9f4fbac31fcc311536dfffd003bfb914580455170a599953fa", Start: 6, End: 16, Line: 1},
	{ID: "F012", Path: "frontend/types.ts", Kind: "symbol", Anchor: "type RowId = string", SHA256: "97a6e15704dd77cfe5860b6dbb75d012acaad94fa116cd1ad76b184b7e5434fc", Start: 7, End: 26, Line: 1},
	{ID: "F013", Path: "frontend/types.ts", Kind: "symbol", Anchor: "interface Row", SHA256: "64ad36b53459466a7049563fbfb7b4b5fb6685b2af68560f700d1d6704361271", Start: 35, End: 48, Line: 2},
	{ID: "F014", Path: "frontend/types.ts", Kind: "symbol", Anchor: "interface Column<T>", SHA256: "d828317118208faa03ae7ff7914f872bc063a4a580d8760d9eea61802b7fc1fc", Start: 86, End: 105, Line: 3},
	{ID: "F015", Path: "frontend/types.ts", Kind: "resolved_type", Anchor: "id: RowId", SHA256: "c3550c9ca5cd105862d8132225a33029587a350ac24aaba515329949987ae43f", Start: 51, End: 60, Line: 2},
	{ID: "F016", Path: "frontend/types.ts", Kind: "resolved_type", Anchor: "readonly Row[]", SHA256: "1695d51219840eba70f1bd7349bcc91299161b9e7a0fdda984f221cd22cba41d", Start: 157, End: 171, Line: 4},
	{ID: "F017", Path: "frontend/utils.ts", Kind: "symbol", Anchor: "function normalize", SHA256: "79a07bef77f013c5a63fd063cfad1947744e135dce4d4a108f98f4e284e4c4f6", Start: 7, End: 25, Line: 1},
	{ID: "F018", Path: "frontend/utils.ts", Kind: "symbol", Anchor: "const lower = normalize", SHA256: "54bbdca52cdb49dcd64c0727dfc36a5e82e5458352225e8d953f21d034c69e38", Start: 95, End: 118, Line: 4},
	{ID: "F019", Path: "frontend/utils.ts", Kind: "unresolved_dynamic", Anchor: "import(modulePath)", SHA256: "cce7cf8609c97a1dd2f326856c3605fd5fb4270bf83090f88765b6181b3f4134", Start: 182, End: 200, Line: 6},
	{ID: "F020", Path: "frontend/utils.ts", Kind: "distinct_positions", Anchor: "return \"same\";", SHA256: "f04ace55918db49aef4b8b3c0677b710e8f42a2c602d2f4c177e9aaa14e37163", Start: 235, End: 249, Line: 8},
	{ID: "F021", Path: "frontend/utils.ts", Kind: "distinct_positions", Anchor: "return \"same\";", SHA256: "f04ace55918db49aef4b8b3c0677b710e8f42a2c602d2f4c177e9aaa14e37163", Start: 288, End: 302, Line: 9},
	{ID: "F022", Path: "frontend/utils.ts", Kind: "no_false_symbol", Anchor: "export function Ghost() {}", SHA256: "46c918acf0a05743ef55ee6f88cbea59f3c1b6af00154365a48a315b49223af3", Start: 326, End: 352, Line: 10},
	{ID: "F023", Path: "frontend/index.ts", Kind: "resolved_reexport", Anchor: "DataTable } from \"./data-table\"", SHA256: "0264cac01040c9a7e7db7728c2baba603f1dcfb743f48e20b495f0576bafb5f4", Start: 9, End: 40, Line: 1},
	{ID: "F024", Path: "frontend/index.ts", Kind: "resolved_reexport", Anchor: "Row, Column } from \"./types\"", SHA256: "3c045a3f89145542a9e608d9b29395115fd166242aba90cef041bbcd3e12cd9e", Start: 56, End: 84, Line: 2},
	{ID: "F025", Path: "frontend/index.ts", Kind: "unresolved_wildcard", Anchor: "export * from \"./types\"", SHA256: "666e3b6d861621f5f2babd23fe861f2961d7a87fc64b861c4a27ff58c4269da0", Start: 86, End: 109, Line: 3},
	{ID: "F026", Path: "backend/table.go", Kind: "symbol", Anchor: "type Row struct", SHA256: "490a3d88e0373cf89e51cf02661b884a5d91280e8f2485ebefeb496710fcd907", Start: 63, End: 78, Line: 4},
	{ID: "F027", Path: "backend/table.go", Kind: "physical_location", Anchor: "imaginary.go:900", SHA256: "2e0ba5747cdbc447dba46b7d6de8bb97222a33c8444f8844ea7e8218615575b5", Start: 45, End: 61, Line: 3},
	{ID: "F028", Path: "backend/table.go", Kind: "method", Anchor: "func (r Row) Display()", SHA256: "922e5b7cc8c4739c81c73ff4c536dbd9f111465e6d9a1c9d532c7166b18c7c43", Start: 108, End: 130, Line: 5},
	{ID: "F029", Path: "backend/table.go", Kind: "resolved_type", Anchor: "(r Row)", SHA256: "e94cdc2a693e0840975674f8a83f26044bf49763bf5d5589d03018cb68c05168", Start: 113, End: 120, Line: 5},
	{ID: "F030", Path: "backend/table.go", Kind: "unresolved_package", Anchor: "\"strings\"", SHA256: "ae55c5b4b4d3f34c5c2eb943366eb54cb73698f321759e1346aae68b414ebee8", Start: 27, End: 36, Line: 2},
	{ID: "F031", Path: "backend/table.go", Kind: "resolved_call", Anchor: "return clean(s)", SHA256: "d6000b31e628b4da707506fb24926f8eb14cd4be9c5bff9c9d5b8173687f0523", Start: 253, End: 268, Line: 7},
	{ID: "F032", Path: "backend/table.go", Kind: "resolved_type", Anchor: "Row{ID: id}", SHA256: "1e63ae9d3bf2fcaede2d5ff935ca56e9b119585dd48045aca59591ad2dd32af4", Start: 308, End: 319, Line: 8},
	{ID: "F033", Path: "backend/table.go", Kind: "unresolved_cross_file", Anchor: "return CrossFile(s)", SHA256: "b2c4bd0d035b2b68004eb656a09085fb83f750c22978d6884f829c03078d6dfc", Start: 357, End: 376, Line: 9},
	{ID: "F034", Path: "backend/table.go", Kind: "no_false_import", Anchor: "\"import ghost\"", SHA256: "4dafe8b8aa75e7efefa91cf0cd7f9ea6794678374be8f2badf5fbc95bd67737f", Start: 391, End: 405, Line: 10},
	{ID: "F035", Path: "backend/types.go", Kind: "symbol", Anchor: "type RemoteRow struct", SHA256: "60b5e9ee8337c8c82cff8250efb003003334f3058989e032d036214c9e70da51", Start: 16, End: 37, Line: 2},
	{ID: "F036", Path: "backend/types.go", Kind: "symbol", Anchor: "func CrossFile", SHA256: "06514eaaa7fa9b0a9a210a2a492fa8e0830a3bde5b492b69f87cf99423a1c70c", Start: 55, End: 69, Line: 3},
	{ID: "F037", Path: "backend/types.go", Kind: "resolved_type", Anchor: "LocalUse(r RemoteRow)", SHA256: "7bf6eab60561cf559eea3f3546fcb26f045d2f0d8f586c7475da1c2cfe993848", Start: 105, End: 126, Line: 4},
	{ID: "F038", Path: "README.md", Kind: "section", Anchor: "# 合成项目", SHA256: "76a3b323129e841a0cc5f89b954d7f631bf8fe307a662e442849030c8c035421", Start: 0, End: 14, Line: 1},
	{ID: "F039", Path: "README.md", Kind: "section", Anchor: "## 前端", SHA256: "98529eb5826d0fc4afde36094979d1c42055e5c1a47d13b3cf8c43916ed32a76", Start: 54, End: 63, Line: 5},
	{ID: "F040", Path: "README.md", Kind: "section", Anchor: "## 后端", SHA256: "291cb49c15643efcfaf2655963b88a6bd10e48083b3a18dbfcaebd4f6f94615d", Start: 130, End: 139, Line: 8},
	{ID: "F041", Path: "README.md", Kind: "no_remote_fetch", Anchor: "https://invalid.example/never-download.png", SHA256: "a1fbb6546b53720d45a9f47b339522d8c9eecf09bf21b1336890e447e46282a7", Start: 203, End: 245, Line: 11},
	{ID: "F042", Path: "README.md", Kind: "small_content", Anchor: "一句话也保留为完整资料。", SHA256: "3af53a23e31de4c9ec48d6fb76fd0fca82ba6e739b1504a42c8475071c44bd74", Start: 16, End: 52, Line: 3},
	{ID: "F043", Path: "docs/guide.md", Kind: "duplicate_heading", Anchor: "## 重复标题", SHA256: "877760b3bb7172e9abd3224690b3b6a31dc51b1639243db948bb1d226f0b3eb6", Start: 18, End: 33, Line: 3},
	{ID: "F044", Path: "docs/guide.md", Kind: "duplicate_heading", Anchor: "## 重复标题", SHA256: "877760b3bb7172e9abd3224690b3b6a31dc51b1639243db948bb1d226f0b3eb6", Start: 145, End: 160, Line: 11},
	{ID: "F045", Path: "docs/guide.md", Kind: "no_false_section", Anchor: "# 围栏中的伪标题", SHA256: "afa80e2452feba9e8ad28e48dbc0468a3bfca3cb6168b49d1854f7259cae0d1d", Start: 69, End: 92, Line: 7},
	{ID: "F046", Path: "docs/guide.md", Kind: "fence_preservation", Anchor: "```tsx", SHA256: "29bd2d2e06a502aebdfcb39a2d3a3bb83989268bf4ee008b7e04363d1224dca8", Start: 61, End: 67, Line: 6},
	{ID: "F047", Path: "docs/guide.md", Kind: "section", Anchor: "Setext 章节\r\n-----------", SHA256: "bd0ebadd65fd5a7f170454e7758efae70c4c2f38ea4ee8800551249554392598", Start: 184, End: 210, Line: 14},
	{ID: "F048", Path: "docs/guide.md", Kind: "table_fidelity", Anchor: "| a | 只保留原文，首版不伪造表格节点 |", SHA256: "98d934d09f0337b9321d2082ad1aa61c4fdfde1383a8d5ffd6a29d7ce1c0335e", Start: 250, End: 303, Line: 19},
	{ID: "F049", Path: "docs/guide.md", Kind: "safe_reading", Anchor: "<img src=", SHA256: "bbf4327372fb95a01969775b4374b15320e011bad304219e77e539c8da23e66e", Start: 307, End: 316, Line: 21},
	{ID: "F050", Path: "docs/guide.md", Kind: "raw_coordinate", Anchor: "第一处正文😀。", SHA256: "3a0d14f7956041bc3f8dd1c9aefc3e7b0aa523cce0b15fbd272d24c5ccac5bce", Start: 35, End: 57, Line: 4},
}

var benchmarkSHA = map[string]string{
	"frontend/data-table.tsx": "3258452b06b6b25931c0e941b496a21de20d04189ed412f7799dc1df7f71d298",
	"frontend/types.ts":       "743c41591799a934b162de5f810bc080f506521a831630ec6a424ecef4375d31",
	"frontend/utils.ts":       "804b49e7535dd8e9ada04da66304a1b6568336ea6a6a13db257c08162b8866bb",
	"frontend/index.ts":       "b290bfff6ef23c5ed8a86cede3df363938bab96494205482b488c85599458181",
	"backend/table.go":        "da691b61207400dc706b65e623c19c4eaae1fdd4be82f9a02b1f7cc73d5229af",
	"backend/types.go":        "0ff304a4d1e9d1aac180a39c97a9791eab19b84a8a867f02b636b5da3e61a987",
	"README.md":               "9a53d29df6a38ffdac2e42c77f4772370085e17ce23b756b2a22761188d8eb29",
	"docs/guide.md":           "0552f0efeee4b159a5cdf6a2a0513cc52b6c9c4380fe083a96b8b76319e424fb",
}
