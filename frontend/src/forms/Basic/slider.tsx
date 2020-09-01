import { Slider, SliderProps, Typography } from "@material-ui/core";
import React from "react";
import { ID } from "utils";
import { FieldProps, getIn } from "formik";

interface Props {
  label?: string;
  min: number;
  max: number;
  step: number;
  disabled?: boolean;
}

// value type is string
export const KFormikRenderSlider = ({
  label,
  min,
  max,
  step,
  disabled,
  field: { name },
  form: { touched, errors, setFieldValue, handleBlur, values },
}: SliderProps & FieldProps & Props) => {
  const id = ID();
  return (
    <div>
      {label ? (
        <Typography id={id} gutterBottom>
          {label}
        </Typography>
      ) : null}
      <Slider
        value={getIn(values, name) || 0}
        onChangeCommitted={(_event: React.ChangeEvent<{}>, value: number | number[]) => setFieldValue(name, value)}
        aria-labelledby={id}
        valueLabelDisplay="auto"
        step={step}
        marks
        min={min}
        max={max}
        disabled={disabled}
      />
    </div>
  );
};
