package metrics

import (
	"errors"
	"github.com/andymeneely/git-churn/gitfuncs"
	"github.com/andymeneely/git-churn/helper"
	"gopkg.in/src-d/go-git.v4"
	"strings"
)

type DiffMetrics struct {
	Insertions  int
	Deletions   int
	LinesBefore int
	LinesAfter  int
}
type FileDiffMetrics struct {
	DiffMetrics
	File       string
	NewFile    bool
	DeleteFile bool
}
type AggrDiffMetrics struct {
	DiffMetrics
	FilesCount   int
	NewFiles     int
	DeletedFiles int
}

func CalculateDiffMetricsWithWhitespace(repo *git.Repository, filePath string) *FileDiffMetrics {
	defer helper.Duration(helper.Track("CalculateDiffMetricsWithWhitespace"))
	diffMetrics := new(FileDiffMetrics)
	diffMetrics.File = filePath
	changes, tree, parentTree := gitfuncs.CommitDiff(repo)
	patch, _ := changes.Patch()
	//fmt.Println(changes)
	//fmt.Println(patch)
	diffStats := patch.Stats()
	//fmt.Println(diffStats)

	//TODO: Throw error if file not exists in this commit
	for _, value := range diffStats {
		if value.Name == filePath {
			diffMetrics.Insertions = value.Addition
			diffMetrics.Deletions = value.Deletion
		}
	}

	diffMetrics.LinesBefore = gitfuncs.FileLOCFromTree(parentTree, filePath)
	diffMetrics.LinesAfter = gitfuncs.FileLOCFromTree(tree, filePath)

	if diffMetrics.LinesBefore == 0 && diffMetrics.LinesAfter != 0 {
		diffMetrics.NewFile = true
	}

	if diffMetrics.LinesBefore != 0 && diffMetrics.LinesAfter == 0 {
		diffMetrics.DeleteFile = true
	}

	return diffMetrics

}

func CalculateDiffMetricsWhitespaceExcluded(repo *git.Repository, filePath string) (*FileDiffMetrics, error) {
	defer helper.Duration(helper.Track("CalculateDiffMetricsWhitespaceExcluded"))
	diffMetrics := new(FileDiffMetrics)
	diffMetrics.File = filePath
	changes, tree, parentTree := gitfuncs.CommitDiff(repo)
	patch, _ := changes.Patch()

	fileDiffTexts := strings.Split(patch.String(), "diff --git a/"+filePath)
	if len(fileDiffTexts) < 2 {
		return nil, errors.New("File: " + filePath + " not found in the given commitHash")
	}
	fileDiff := strings.Split(fileDiffTexts[1], "+++")[1]
	fileDiff = strings.Split(fileDiff, "diff --git")[0]
	lines := strings.Split(fileDiff, "\n")

	insertions := 0
	deletions := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "+") && line != "+" {
			insertions += 1
		}
		if strings.HasPrefix(line, "-") && line != "-" {
			deletions += 1
		}
	}

	diffMetrics.Insertions = insertions
	diffMetrics.Deletions = deletions

	diffMetrics.LinesBefore = gitfuncs.FileLOCFromTreeWhitespaceExcluded(parentTree, filePath)
	diffMetrics.LinesAfter = gitfuncs.FileLOCFromTreeWhitespaceExcluded(tree, filePath)

	if diffMetrics.LinesBefore == 0 && diffMetrics.LinesAfter != 0 {
		diffMetrics.NewFile = true
	}

	if diffMetrics.LinesBefore != 0 && diffMetrics.LinesAfter == 0 {
		diffMetrics.DeleteFile = true
	}

	return diffMetrics, nil
}

//Gets the aggregated DiffMetrics for all the files in the given repo for the specified commit hash.
//It includes the whitespaces while counting the changes.
func AggrDiffMetricsWithWhitespace(repo *git.Repository) *AggrDiffMetrics {
	defer helper.Duration(helper.Track("AggrDiffMetricsWithWhitespace"))
	diffMetrics := new(AggrDiffMetrics)
	changes, tree, parentTree := gitfuncs.CommitDiff(repo)
	patch, _ := changes.Patch()
	//fmt.Println(changes)
	//fmt.Println(patch)
	diffStats := patch.Stats()
	//fmt.Println(diffStats)

	additions := 0
	deletions := 0
	for _, value := range diffStats {
		additions += value.Addition
		deletions += value.Deletion
	}
	diffMetrics.Insertions = additions
	diffMetrics.Deletions = deletions

	var beforeFiles []string
	var afterFiles []string
	beforeCh := make(chan func() (int, []string))
	go gitfuncs.LOCFilesFromTree(parentTree, beforeCh)

	afterCh := make(chan func() (int, []string))
	go gitfuncs.LOCFilesFromTree(tree, afterCh)
	diffMetrics.LinesBefore, beforeFiles = (<-beforeCh)()
	diffMetrics.LinesAfter, afterFiles = (<-afterCh)()

	setFilesCounts(beforeFiles, afterFiles, diffMetrics)
	return diffMetrics
}

//Sets the count of new files, deleted files and total fines count
func setFilesCounts(beforeFiles []string, afterFiles []string, diffMetrics *AggrDiffMetrics) {
	diffMetrics.FilesCount = len(afterFiles)

	deletedFiles := make(chan int)
	newFiles := make(chan int)

	go getNewFilesCount(beforeFiles, afterFiles, newFiles)
	go getDeletedFilesCount(beforeFiles, afterFiles, deletedFiles)

	diffMetrics.NewFiles = <-newFiles
	diffMetrics.DeletedFiles = <-deletedFiles
}

func getDeletedFilesCount(beforeFiles []string, afterFiles []string, deletedFiles chan int) {
	// Putting the file names in a map to make lookup faster
	count := 0
	afterSet := make(map[string]bool)
	for _, f := range afterFiles {
		afterSet[f] = true
	}

	for _, file := range beforeFiles {
		if !afterSet[file] {
			count += 1
		}
	}
	deletedFiles <- count
}

func getNewFilesCount(beforeFiles []string, afterFiles []string, newFiles chan int) {
	// Putting the file names in a map to make lookup faster
	beforeSet := make(map[string]bool)
	count := 0
	for _, f := range beforeFiles {
		beforeSet[f] = true
	}

	for _, file := range afterFiles {
		if !beforeSet[file] {
			count += 1
		}
	}
	newFiles <- count
}

//Gets the aggregated DiffMetrics for all the files in the given repo for the specified commit hash.
//It neglects the whitespaces while counting the changes
func AggrDiffMetricsWhitespaceExcluded(repo *git.Repository) (*AggrDiffMetrics, error) {
	defer helper.Duration(helper.Track("AggrDiffMetricsWhitespaceExcluded"))
	diffMetrics := new(AggrDiffMetrics)
	changes, tree, parentTree := gitfuncs.CommitDiff(repo)
	patch, _ := changes.Patch()

	fileDiffTexts := strings.Split(patch.String(), "diff --git a/")
	insertions := 0
	deletions := 0
	for index, _ := range fileDiffTexts {
		if index == 0 {
			continue
		}
		fileDiff := strings.Split(fileDiffTexts[index], "+++")[1]
		fileDiff = strings.Split(fileDiff, "diff --git")[0]
		lines := strings.Split(fileDiff, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)

			if strings.HasPrefix(line, "+") && line != "+" {
				insertions += 1
			}
			if strings.HasPrefix(line, "-") && line != "-" {
				deletions += 1
			}
		}
	}

	diffMetrics.Insertions = insertions
	diffMetrics.Deletions = deletions

	var beforeFiles []string
	var afterFiles []string
	diffMetrics.LinesBefore, beforeFiles = gitfuncs.LOCFilesFromTreeWhitespaceExcluded(parentTree)
	diffMetrics.LinesAfter, afterFiles = gitfuncs.LOCFilesFromTreeWhitespaceExcluded(tree)

	setFilesCounts(beforeFiles, afterFiles, diffMetrics)
	return diffMetrics, nil
}
