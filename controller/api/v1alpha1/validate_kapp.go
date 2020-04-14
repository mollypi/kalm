package v1alpha1

type KappValidateError struct {
	err string
	path string
}

func (k KappValidateError) Error() string {
	return k.err
}

func TryValidateApplication(appSpec ApplicationSpec) error {
	// for now only check dependency here
	validateFuncs := []func(spec ApplicationSpec) error{isValidateDependency}

	for _, validateFunc := range validateFuncs {
		if err := validateFunc(appSpec); err != nil {
			return err
		}
	}

	return nil
}
