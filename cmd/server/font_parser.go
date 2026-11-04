package main

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// FontMetadata 存放从字体文件中解析出的元数据
type FontMetadata struct {
	DisplayName string   // 友好展示名称，如 "Arial"、"仿宋 (FangSong)"
	Family      string   // 字体族名
	Subfamily   string   // 样式/字重（如 Regular, Bold）
	FullName    string   // 完整名称
	PostScript  string   // PostScript 名称
	Format      string   // 格式，如 "TTF", "OTF", "TTC"
	AllFamilies []string // 解析出的所有族名变体（包含中英文、别名，用于系统识别）
}

// fontNameEntry 单条字体名称记录
type fontNameEntry struct {
	familyZh      string
	familyEn      string
	subfamily     string
	fullNameZh    string
	fullNameEn    string
	postScript    string
	preferredZh   string
	preferredEn   string
}

func (e *fontNameEntry) bestFamily() string {
	famZh := e.preferredZh
	if famZh == "" {
		famZh = e.familyZh
	}
	famEn := e.preferredEn
	if famEn == "" {
		famEn = e.familyEn
	}

	if famZh != "" && famEn != "" && !strings.EqualFold(famZh, famEn) {
		return famZh + " (" + famEn + ")"
	}
	if famZh != "" {
		return famZh
	}
	if famEn != "" {
		return famEn
	}
	if e.fullNameZh != "" {
		return e.fullNameZh
	}
	if e.fullNameEn != "" {
		return e.fullNameEn
	}
	if e.postScript != "" {
		return e.postScript
	}
	return ""
}

// ParseFontMetadataFromFile 从本地字体文件中解析元数据
func ParseFontMetadataFromFile(filePath string) (FontMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return fallbackFontMetadata(filePath), err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fallbackFontMetadata(filePath), err
	}

	meta, err := parseFontMetadataFromReader(file, stat.Size(), filepath.Base(filePath))
	if err != nil {
		return fallbackFontMetadata(filePath), err
	}
	return meta, nil
}

func fallbackFontMetadata(filePath string) FontMetadata {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	format := strings.ToUpper(strings.TrimPrefix(ext, "."))
	if format == "" {
		format = "FONT"
	}
	return FontMetadata{
		DisplayName: nameWithoutExt,
		Family:      nameWithoutExt,
		Subfamily:   "Regular",
		FullName:    nameWithoutExt,
		Format:      format,
		AllFamilies: []string{nameWithoutExt},
	}
}

func parseFontMetadataFromReader(r io.ReaderAt, fileSize int64, filename string) (FontMetadata, error) {
	header := make([]byte, 12)
	if _, err := r.ReadAt(header, 0); err != nil {
		return fallbackFontMetadata(filename), err
	}

	magic := binary.BigEndian.Uint32(header[0:4])
	var format string
	var offsets []uint32

	switch magic {
	case 0x74746366: // 'ttcf' (TrueType Collection)
		format = "TTC"
		numFonts := binary.BigEndian.Uint32(header[8:12])
		if numFonts > 256 {
			numFonts = 256
		}
		offsetBuf := make([]byte, numFonts*4)
		if _, err := r.ReadAt(offsetBuf, 12); err != nil {
			return fallbackFontMetadata(filename), err
		}
		for i := uint32(0); i < numFonts; i++ {
			off := binary.BigEndian.Uint32(offsetBuf[i*4 : (i+1)*4])
			if int64(off) < fileSize {
				offsets = append(offsets, off)
			}
		}
	case 0x00010000, 0x74727565: // TrueType 1.0 or 'true'
		format = "TTF"
		offsets = []uint32{0}
	case 0x4F54544F: // 'OTTO' (OpenType with CFF)
		format = "OTF"
		offsets = []uint32{0}
	default:
		// 根据文件扩展名兜底
		ext := strings.ToLower(filepath.Ext(filename))
		switch ext {
		case ".ttc":
			format = "TTC"
		case ".otf":
			format = "OTF"
		default:
			format = "TTF"
		}
		offsets = []uint32{0}
	}

	var entries []fontNameEntry
	var allFamilies []string

	for _, off := range offsets {
		entry, err := parseSingleFontNameTable(r, fileSize, int64(off))
		if err == nil {
			entries = append(entries, entry)
			if entry.familyZh != "" {
				allFamilies = append(allFamilies, entry.familyZh)
			}
			if entry.familyEn != "" {
				allFamilies = append(allFamilies, entry.familyEn)
			}
			if entry.preferredZh != "" {
				allFamilies = append(allFamilies, entry.preferredZh)
			}
			if entry.preferredEn != "" {
				allFamilies = append(allFamilies, entry.preferredEn)
			}
			if entry.fullNameZh != "" {
				allFamilies = append(allFamilies, entry.fullNameZh)
			}
			if entry.fullNameEn != "" {
				allFamilies = append(allFamilies, entry.fullNameEn)
			}
		}
	}

	if len(entries) == 0 {
		return fallbackFontMetadata(filename), nil
	}

	// 组合展示名称
	var displayNames []string
	seen := make(map[string]bool)
	for _, e := range entries {
		bf := e.bestFamily()
		if bf != "" && !seen[bf] {
			seen[bf] = true
			displayNames = append(displayNames, bf)
		}
	}

	displayName := strings.Join(displayNames, " / ")
	if displayName == "" {
		displayName = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}

	primary := entries[0]
	family := primary.preferredZh
	if family == "" {
		family = primary.familyZh
	}
	if family == "" {
		family = primary.preferredEn
	}
	if family == "" {
		family = primary.familyEn
	}
	if family == "" {
		family = displayName
	}

	subfamily := primary.subfamily
	if subfamily == "" {
		subfamily = "Regular"
	}

	fullName := primary.fullNameZh
	if fullName == "" {
		fullName = primary.fullNameEn
	}
	if fullName == "" {
		fullName = displayName
	}

	return FontMetadata{
		DisplayName: displayName,
		Family:      family,
		Subfamily:   subfamily,
		FullName:    fullName,
		PostScript:  primary.postScript,
		Format:      format,
		AllFamilies: uniqueStrings(allFamilies),
	}, nil
}

func parseSingleFontNameTable(r io.ReaderAt, fileSize int64, fontStart int64) (fontNameEntry, error) {
	var entry fontNameEntry
	sfntHeader := make([]byte, 12)
	if _, err := r.ReadAt(sfntHeader, fontStart); err != nil {
		return entry, err
	}

	numTables := int(binary.BigEndian.Uint16(sfntHeader[4:6]))
	if numTables > 256 {
		numTables = 256
	}

	tableRecordSize := 16
	tableDirBytes := make([]byte, numTables*tableRecordSize)
	if _, err := r.ReadAt(tableDirBytes, fontStart+12); err != nil {
		return entry, err
	}

	var nameTableOffset uint32
	var nameTableLength uint32
	for i := 0; i < numTables; i++ {
		rec := tableDirBytes[i*tableRecordSize : (i+1)*tableRecordSize]
		tag := binary.BigEndian.Uint32(rec[0:4])
		if tag == 0x6E616D65 { // 'name'
			nameTableOffset = binary.BigEndian.Uint32(rec[8:12])
			nameTableLength = binary.BigEndian.Uint32(rec[12:16])
			break
		}
	}

	if nameTableOffset == 0 || nameTableLength == 0 || int64(nameTableOffset+nameTableLength) > fileSize {
		return entry, io.ErrUnexpectedEOF
	}

	// 读取 name 表内容（一般 name 表只有几 KB 到几十 KB）
	nameData := make([]byte, nameTableLength)
	if _, err := r.ReadAt(nameData, int64(nameTableOffset)); err != nil {
		return entry, err
	}

	if len(nameData) < 6 {
		return entry, io.ErrUnexpectedEOF
	}

	count := int(binary.BigEndian.Uint16(nameData[2:4]))
	stringOffset := int(binary.BigEndian.Uint16(nameData[4:6]))
	if count > 1000 {
		count = 1000
	}

	const nameRecordSize = 12
	if len(nameData) < 6+count*nameRecordSize {
		return entry, io.ErrUnexpectedEOF
	}

	for i := 0; i < count; i++ {
		rec := nameData[6+i*nameRecordSize : 6+(i+1)*nameRecordSize]
		platformID := binary.BigEndian.Uint16(rec[0:2])
		encodingID := binary.BigEndian.Uint16(rec[2:4])
		languageID := binary.BigEndian.Uint16(rec[4:6])
		nameID := binary.BigEndian.Uint16(rec[6:8])
		length := int(binary.BigEndian.Uint16(rec[8:10]))
		offset := int(binary.BigEndian.Uint16(rec[10:12]))

		strStart := stringOffset + offset
		strEnd := strStart + length
		if strStart < 0 || strEnd > len(nameData) || length <= 0 {
			continue
		}

		rawBytes := nameData[strStart:strEnd]
		str := decodeFontString(platformID, encodingID, rawBytes)
		str = strings.TrimSpace(str)
		if str == "" {
			continue
		}

		// 判定是否是中文语言（zh-CN, zh-TW, zh-HK）
		isChinese := (platformID == 3 && (languageID == 0x0804 || languageID == 0x0404 || languageID == 0x0C04)) || containsHan(str)
		isEnglish := (platformID == 3 && languageID == 0x0409) || (platformID == 1 && encodingID == 0)

		switch nameID {
		case 1: // Font Family
			if isChinese && entry.familyZh == "" {
				entry.familyZh = str
			} else if isEnglish && entry.familyEn == "" {
				entry.familyEn = str
			} else if entry.familyEn == "" && entry.familyZh == "" {
				entry.familyEn = str
			}
		case 2: // Subfamily (Weight / Style)
			if entry.subfamily == "" || isEnglish {
				entry.subfamily = str
			}
		case 4: // Full Font Name
			if isChinese && entry.fullNameZh == "" {
				entry.fullNameZh = str
			} else if isEnglish && entry.fullNameEn == "" {
				entry.fullNameEn = str
			}
		case 6: // PostScript Name
			if entry.postScript == "" {
				entry.postScript = str
			}
		case 16: // Typographic Family Name (Preferred Family)
			if isChinese && entry.preferredZh == "" {
				entry.preferredZh = str
			} else if isEnglish && entry.preferredEn == "" {
				entry.preferredEn = str
			}
		}
	}

	return entry, nil
}

func decodeFontString(platformID, encodingID uint16, b []byte) string {
	// Platform 0 (Unicode) 或 Platform 3 (Windows) 多采用 UTF-16BE 编码
	if platformID == 0 || platformID == 3 {
		if len(b)%2 == 0 {
			u16s := make([]uint16, len(b)/2)
			for i := range u16s {
				u16s[i] = binary.BigEndian.Uint16(b[i*2:])
			}
			return string(utf16.Decode(u16s))
		}
	}
	// Platform 1 (Macintosh) 英文 ASCII
	if platformID == 1 && encodingID == 0 {
		return string(b)
	}

	// 尝试 ASCII
	isAscii := true
	for _, c := range b {
		if c == 0 || c >= 0x80 {
			isAscii = false
			break
		}
	}
	if isAscii {
		return string(b)
	}

	// 尝试 UTF-16BE
	if len(b)%2 == 0 {
		u16s := make([]uint16, len(b)/2)
		for i := range u16s {
			u16s[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		s := string(utf16.Decode(u16s))
		if utf8ValidAndReadable(s) {
			return s
		}
	}

	return string(b)
}

func containsHan(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func utf8ValidAndReadable(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r == 0 || r == 0xFFFD {
			return false
		}
	}
	return true
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range input {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
