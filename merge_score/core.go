package merge_score

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/sirupsen/logrus"

	"github.com/elonzh/skr/pkg/utils"
)

const (
	sheetIndex = 1
)

//  数据表样例如下:
//   ___________________________________________________
// 1|    国际学院20xx级xxx班20xx-20xx学年第x学期成绩统计   |
// 2|    学生信息      |  语言课程分数   |  专业课程分数   |
// 3| 序号 姓名 语言班 | 语言课1 语言课2 | 专业课1 专业课2 |
//     A   B     C       D      E         F      G
func LoadScoreTable(file *excelize.File, skipRows uint32) (*ScoreTable, error) {
	logrus.WithFields(logrus.Fields{
		"SheetMap": file.GetSheetMap(),
		"Path":     file.Path,
	}).Debugln("开始加载数据")
	rows, err := file.GetRows(file.GetSheetName(sheetIndex))
	if err != nil {
		return nil, err
	}
	rows = rows[skipRows:]
	headers := rows[0]
	for i, v := range headers {
		headers[i] = strings.TrimSpace(v)
		if headers[i] == "" {
			logrus.WithFields(logrus.Fields{
				"文件": file.Path,
				"表头": headers,
			}).Errorln("存在空白表头")
			return nil, fmt.Errorf(`文件 "%s" 存在空白表头，表头: %v`, file.Path, headers)
		}
	}
	if len(headers) < 4 {
		logrus.WithFields(logrus.Fields{
			"文件": file.Path,
			"表头": headers,
		}).Errorln("列数小于 4 列，是不是没有数据？")
		return nil, fmt.Errorf(`文件 "%s" 列数小于 4 列，是不是没有数据？表头: %v`, file.Path, headers)
	}
	studentNum := len(rows) - 1
	logrus.WithFields(logrus.Fields{
		"SheetMap":      file.GetSheetMap(),
		"Path":          file.Path,
		"HeadersLength": len(headers),
		"Headers":       headers,
		"StudentNum":    studentNum,
	}).Debugln("headers loaded")

	const subjectNameIndexOffset = 3
	t := &ScoreTable{
		file:     file,
		SkipRows: skipRows,
		ScoreMap: make(map[StudentSubject]StudentSubjectScore, studentNum*(len(headers)-subjectNameIndexOffset)),
	}
	for rowIndex, row := range rows[1:] {
		x := rowIndex + int(skipRows) + 1 + 1
		if len(row) < subjectNameIndexOffset {
			logrus.WithFields(logrus.Fields{
				"文件": file.Path,
				"行":  x,
			}).Warnln("数据缺失, 跳过该行")
			continue
		}
		if len(row) > len(headers) {
			row = row[:len(headers)]
		}
		className := strings.TrimSpace(row[2])
		studentName := strings.TrimSpace(row[1])
		if className == "" || studentName == "" {
			logrus.WithFields(logrus.Fields{
				"文件": file.Path,
				"行":  x,
				"班级": className,
				"学生": studentName,
			}).Warnln("学生信息缺失, 跳过该行")
			continue
		}
		// 先初始化学生的成绩表
		for subjectNameIndex, subjectName := range headers[subjectNameIndexOffset:] {
			y := subjectNameIndex + subjectNameIndexOffset + 1
			t.ScoreMap[StudentSubject{
				ClassName:   className,
				StudentName: studentName,
				SubjectName: subjectName,
			}] = StudentSubjectScore{
				ScoreData: "",
				X:         x,
				Y:         y,
			}
		}
		for subjectNameIndex, rawScoreStr := range row[subjectNameIndexOffset:] {
			y := subjectNameIndex + subjectNameIndexOffset + 1
			subjectName := headers[y-1]
			if subjectName == "" {
				logrus.WithFields(logrus.Fields{
					"文件": file.Path,
					"行":  x,
					"列":  MustColumnNumberToName(y),
					"班级": className,
					"学生": studentName,
				}).Warnln("科目缺失, 跳过该列")
				continue
			}
			rawScoreStr = strings.TrimSpace(rawScoreStr)
			if rawScoreStr == "" {
				t.ScoreMap[StudentSubject{
					ClassName:   className,
					StudentName: studentName,
					SubjectName: headers[subjectNameIndex+subjectNameIndexOffset],
				}] = StudentSubjectScore{
					ScoreData: "",
					X:         x,
					Y:         y,
				}
				logrus.WithFields(logrus.Fields{
					"文件":   file.Path,
					"行":    x,
					"列":    MustColumnNumberToName(y),
					"班级":   className,
					"学生":   studentName,
					"科目":   subjectName,
					"分数数据": rawScoreStr,
				}).Warnln("分数数据为空")
			} else {
				t.ScoreMap[StudentSubject{
					ClassName:   className,
					StudentName: studentName,
					SubjectName: subjectName,
				}] = StudentSubjectScore{
					ScoreData: rawScoreStr,
					X:         x,
					Y:         y,
				}
				logrus.WithFields(logrus.Fields{
					"文件":   file.Path,
					"行":    x,
					"列":    MustColumnNumberToName(y),
					"班级":   className,
					"学生":   studentName,
					"科目":   subjectName,
					"分数数据": rawScoreStr,
				}).Debugln("分数加载成功")
			}
		}
	}
	return t, nil
}

func LoadScoreTableFromPath(path string, skipRows uint32) (*ScoreTable, error) {
	file, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	t, err := LoadScoreTable(file, skipRows)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func MergeScore(resultFilePath string, scoreFilePaths []string, skipRows uint32) error {
	if len(scoreFilePaths) == 0 {
		return errors.New("should provide at least one scoreFilePath")
	}

	file, err := excelize.OpenFile(resultFilePath)
	if err != nil {
		return err
	}
	resultScoreTable, err := LoadScoreTable(file, skipRows)
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"文件":    file.Path,
		"学生科目数": len(resultScoreTable.ScoreMap),
		"学生科目表": len(resultScoreTable.ScoreMap),
	}).Infoln("读取汇总表成功")
	sheetName := file.GetSheetName(sheetIndex)

	scoreTables := make([]*ScoreTable, 0, len(scoreFilePaths))
	for _, scoreFilePath := range scoreFilePaths {
		fileInfo, err := os.Stat(scoreFilePath)
		if err != nil {
			return err
		}
		if fileInfo.IsDir() {
			err := filepath.Walk(scoreFilePath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return err
				}
				t, err := LoadScoreTableFromPath(path, skipRows)
				if err != nil {
					return err
				}
				scoreTables = append(scoreTables, t)
				return nil
			})
			if err != nil {
				return err
			}
		} else {
			t, err := LoadScoreTableFromPath(scoreFilePath, skipRows)
			if err != nil {
				return err
			}
			scoreTables = append(scoreTables, t)
		}
	}

	for subject, score := range resultScoreTable.ScoreMap {
		for _, scoreTable := range scoreTables {
			realScore, ok := scoreTable.ScoreMap[subject]
			if ok && realScore.ScoreData != "" {
				s := StudentSubjectScore{
					ScoreData: realScore.ScoreData,
					X:         score.X,
					Y:         score.Y,
				}
				resultScoreTable.ScoreMap[subject] = s
				rawScore, err := strconv.ParseFloat(s.ScoreData, 64)
				if err != nil {
					if err := file.SetCellStr(sheetName, s.GetAxis(), s.ScoreData); err != nil {
						logrus.WithError(err).Warningln("分数设置错误")
					}
					break
				} else {
					// 汇总成绩四舍五入保留整数
					if err := file.SetCellInt(sheetName, s.GetAxis(), int(math.Round(rawScore))); err != nil {
						logrus.WithError(err).Warningln("分数设置错误")
					}
					break
				}
			}
		}
	}
	outputPath := utils.PrefixedPath("已汇总-", file.Path)
	err = file.SaveAs(outputPath)
	if err != nil {
		return err
	}
	logrus.WithField("汇总表位置", outputPath).Info("成绩汇总完成")
	return nil
}
