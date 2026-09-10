package cli

import "fmt"

func cmdReview(e *env, args []string) error {
	fs := newFlagSet("review")
	_ = commonFlags(fs, e)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if err := atMostArgs("review", rest, 0); err != nil {
		return err
	}
	return fmt.Errorf("keel review 尚未实现（计划在 M3：受控进化）。" +
		"现在可用：keel why 查历史、keel check 验证、keel brief 取上下文")
}
