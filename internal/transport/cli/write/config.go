package write

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type fieldSet map[string]any

func resolve(opts options) (options, error) {
	var fromFile options

	if opts.configPath != "" {
		loaded, err := loadSettings(opts.configPath)
		if err != nil {
			return options{}, err
		}

		fromFile = loaded
	}

	opts.authorName = firstSet(opts.authorName, fromFile.authorName)
	opts.authorEmail = firstSet(opts.authorEmail, fromFile.authorEmail)
	opts.dateRef = firstSet(opts.dateRef, fromFile.dateRef)
	opts.subjects = firstSet(opts.subjects, fromFile.subjects)

	if opts.authorName == "" || opts.authorEmail == "" {
		return options{}, fmt.Errorf("%w: an identity needs a name and an email", ErrUsage)
	}

	if err := checkIdentity(opts.authorName, opts.authorEmail); err != nil {
		return options{}, err
	}

	if opts.dateRef == "" {
		return options{}, fmt.Errorf("%w: --date-ref is required", ErrUsage)
	}

	if err := errors.Join(
		checkRev("--date-ref", opts.dateRef), checkRev("--verify", opts.verify),
	); err != nil {
		return options{}, err
	}

	return opts, nil
}

func firstSet(flagValue, fileValue string) string {
	if flagValue != "" {
		return flagValue
	}

	return fileValue
}

func loadSettings(path string) (options, error) {
	file, err := bootstrap.LoadConfigFile(path)
	if err != nil {
		return options{}, err //nolint:wrapcheck // a config error names its file once
	}

	var settings options

	if len(file.File.Write) == 0 {
		return settings, nil
	}

	fields := fieldSet{
		"identity": fieldSet{"name": &settings.authorName, "email": &settings.authorEmail},
		"dateRef":  &settings.dateRef,
		"subjects": &settings.subjects,
	}

	if err = decodeFields(file.File.Write, fields); err != nil {
		return options{}, fmt.Errorf("%w: %s: write: %w", bootstrap.ErrConfig, path, err)
	}

	return settings, nil
}

func decodeFields(raw json.RawMessage, fields fieldSet) error {
	var members map[string]json.RawMessage
	if err := rules.DecodeStrict(raw, &members, "an object"); err != nil {
		return err //nolint:wrapcheck // the caller names the file and section
	}

	for _, key := range slices.Sorted(maps.Keys(members)) {
		var err error

		switch target := fields[key].(type) {
		case fieldSet:
			err = decodeFields(members[key], target)
		case *string:
			err = rules.DecodeStrict(members[key], target, "a string")
		default:
			err = fmt.Errorf("unknown key %q", key)
		}

		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}

	return nil
}

func checkIdentity(name, email string) error {
	identity := name + email

	if strings.ContainsAny(identity, "<>") || strings.IndexFunc(identity, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: an identity cannot hold a control byte, '<' or '>'", ErrUsage)
	}

	return nil
}

func checkRef(ref string) error {
	if err := gitstore.CheckRef(ref); err != nil {
		return fmt.Errorf("%w: --ref %q is not a valid ref name", ErrUsage, ref)
	}

	return nil
}

func checkRev(flag, rev string) error {
	if strings.HasPrefix(rev, "-") {
		return fmt.Errorf("%w: %s %q may not start with a dash", ErrUsage, flag, rev)
	}

	return nil
}
