package runtime

const UpdateKind = "update"

type Update struct {
	Value    any
	Visible  *bool
	Disabled *bool
	Choices  []string
	Label    *string
}

func Value(value any) Update {
	return Update{Value: value}
}

func Visible(value bool) Update {
	return Update{Visible: &value}
}

func Disabled(value bool) Update {
	return Update{Disabled: &value}
}

func Choices(values ...string) Update {
	return Update{Choices: append([]string{}, values...)}
}

func Label(value string) Update {
	return Update{Label: &value}
}
