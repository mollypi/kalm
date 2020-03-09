import { Button, createStyles, Grid, WithStyles, withStyles, Paper } from "@material-ui/core";
import { Theme } from "@material-ui/core/styles";
import Typography from "@material-ui/core/Typography";
import Immutable from "immutable";
import React from "react";
import { connect, DispatchProp } from "react-redux";
import { InjectedFormProps } from "redux-form";
import { Field, formValueSelector, getFormValues, reduxForm } from "redux-form/immutable";
import { Application, ComponentTemplate, SharedEnv } from "../../actions";
import { RootState } from "../../reducers";
import { HelperContainer } from "../../widgets/Helper";
import { SwitchField } from "../Basic/switch";
import { TextField } from "../Basic/text";
import { NormalizeBoolean } from "../normalizer";
import { ValidatorRequired } from "../validator";
import { Components } from "./component";
import { SharedEnvs } from "./shardEnv";

const styles = (theme: Theme) =>
  createStyles({
    formSection: {
      padding: theme.spacing(2),
      margin: theme.spacing(3)
    }
  });

const mapStateToProps = (state: RootState) => {
  const selector = formValueSelector("application");
  const formComponents: ComponentTemplate[] = selector(state, "components");
  const sharedEnv: Immutable.List<SharedEnv> = selector(state, "sharedEnv");
  const values = getFormValues("application")(state) as Application;

  return {
    sharedEnv,
    formComponents,
    values
  };
};

export interface Props {}

class ApplicationFormRaw extends React.PureComponent<
  Props &
    InjectedFormProps<Application, Props> &
    ReturnType<typeof mapStateToProps> &
    WithStyles<typeof styles> &
    DispatchProp
> {
  private getIsEdit() {
    return !!this.props.values.get("resourceVersion");
  }

  private renderBaisc() {
    // const { classes } = this.props;
    const isEdit = this.getIsEdit();
    return (
      <>
        <HelperContainer>
          <Typography>Basic information of this application</Typography>
        </HelperContainer>

        <Grid container spacing={2}>
          <Grid item md={6}>
            <Field
              name="name"
              label="Name"
              disabled={isEdit}
              component={TextField}
              validate={ValidatorRequired}
              helperText={
                isEdit
                  ? "Can't modify name"
                  : 'The characters allowed in names are: digits (0-9), lower case letters (a-z), "-", and ".". Max length is 180.'
              }
              placeholder="Please type the component name"
            />
          </Grid>
          <Grid item md={6}>
            <Field
              name="namespace"
              label="Namespace"
              disabled={isEdit}
              component={TextField}
              validate={ValidatorRequired}
              helperText={isEdit ? "Can't modify namespace" : "All resources will running in this namespace."}
              placeholder="Please type the namespace"
            />
          </Grid>
        </Grid>
        <div>
          <Field
            name="isPersistent"
            formControlLabelProps={{
              label: "Persistent"
            }}
            disabled={isEdit}
            component={SwitchField}
            normalizer={NormalizeBoolean}
            tooltipProps={{
              title:
                "This option controls how disks are mounted. " +
                "If true, the system will use persistent disks as you defined. Data won't lost during restart. It's suitable for a production deployment." +
                "If false, it will use temporary disks, data will be lost during a restart. You should only use this mode in test case."
            }}
          />
        </div>
        <Field
          name="isActive"
          formControlLabelProps={{
            label: "Active"
          }}
          component={SwitchField}
          normalizer={NormalizeBoolean}
        />
      </>
    );
  }

  private renderComponent() {
    return (
      <>
        <HelperContainer>
          <Typography>Select compoents you want to include into this application.</Typography>
        </HelperContainer>
        <Components />
      </>
    );
  }

  private renderSharedEnvs() {
    return (
      <>
        <HelperContainer>
          <Typography>Shared environment variable is consistent amoung all components.</Typography>
        </HelperContainer>
        <SharedEnvs />
      </>
    );
  }

  public render() {
    const { handleSubmit, classes } = this.props;

    // const tabs = [
    //   {
    //     title: "Basic Info",
    //     component: this.renderBaisc()
    //   },
    //   {
    //     title: "Components",
    //     component: this.renderComponent()
    //   },
    //   {
    //     title: "Shared Envs",
    //     component: this.renderSharedEnvs()
    //   }
    // ];

    return (
      <form onSubmit={handleSubmit} style={{ height: "100%", overflow: "hidden" }}>
        <Paper className={classes.formSection}>{this.renderBaisc()}</Paper>
        <Paper className={classes.formSection}>{this.renderComponent()}</Paper>
        <Paper className={classes.formSection}>{this.renderSharedEnvs()}</Paper>
        <Button variant="contained" color="primary" type="submit">
          Submit
        </Button>
      </form>
    );
  }
}

const initialValues: Application = Immutable.fromJS({
  id: "0",
  name: "a-sample-application",
  sharedEnv: [],
  components: []
});

export default reduxForm<Application, Props>({
  form: "application",
  initialValues,
  onSubmitFail: (...args) => {
    console.log("submit failed", args);
  }
})(connect(mapStateToProps)(withStyles(styles)(ApplicationFormRaw)));
