#!/bin/bash

# allow a specific version to be passed in as the first argument, for example:
# ./script/install-local-mode.sh vx.x.x

default_version=df8af36f884fa42e568a94564668f5d7006804c5
KALM_VERSION=${1:-$default_version}

echo "Installing Kalm $KALM_VERSION"
echo ""

CUR_DIR=$(dirname -- "$0")

# set kalm version in kalmoperator yaml
sed -E "s/KALM_VERSION_PLACEHOLDER/${KALM_VERSION}/g" $CUR_DIR/template-kalm-install-operator.yaml | kubectl apply -f -

CRD_ESTABLISHED=""
CRD_NAMES_ACCEPTED=""

while [ "$CRD_ESTABLISHED" != "True" ] || [ "$CRD_NAMES_ACCEPTED" != "True" ]
do
  echo -e "\nAwaiting installation of CRDs"
  #sleep 1

  CRD_ESTABLISHED=$(kubectl get crd kalmoperatorconfigs.install.kalm.dev -ojsonpath='{.status.conditions[?(@.type=="Established")].status}')
  CRD_NAMES_ACCEPTED=$(kubectl get crd kalmoperatorconfigs.install.kalm.dev -ojsonpath='{.status.conditions[?(@.type=="NamesAccepted")].status}')
done

OPERATOR_CONFIG_APPLY_STATUS=1

while [ "$OPERATOR_CONFIG_APPLY_STATUS" -ne 0 ]
do
  # set kalm version in kalmoperatorconfig yaml
  sed -E "s/KALM_VERSION_PLACEHOLDER/${KALM_VERSION}/g" $CUR_DIR/template-kalm-install-kalmoperatorconfig.yaml | kubectl apply -f -

  OPERATOR_CONFIG_APPLY_STATUS=$?
done

sleep 1

finish=False

function printCompletedItems() {
  Green='\033[0;32m'
  OFF='\033[0m'

  clear
  echo "Initializing Kalm - ${#installed[@]}/${#ns_array[@]} modules ready:"
  echo ""

  for i in "${installed[@]}"
  do
    echo -e "${Green}✔${OFF} $i"
  done
}

while [ $finish != "True" ]
do
    installing=()
    installed=()

    ns_array=(kalm-operator cert-manager istio-system kalm-system)
    dp_cnt_array=(1 3 3 2)

    #for ns in ns_array
    for i in ${!ns_array[@]}
    do
        ns=${ns_array[i]}
        dp_cnt=${dp_cnt_array[i]}

        dp_status_output=$(kubectl get deployments.apps -n $ns -ojsonpath='{.items[*].status.conditions[?(@.type=="Available")].status}')
        dp_status_list=($dp_status_output)

        status_list_size=${#dp_status_list[@]}

        #echo $dp_status_output, ${dp_status_list[@]} $status_list_size

        if [ $status_list_size -lt $dp_cnt ]
        then
            installing+=($ns)
        else
            for status in $dp_status_list
            do
                if [ $status == "True" ]
                then
                    installed+=($ns)
                    continue
                fi

                installing+=($ns)
                break
            done
        fi
    done

    installing_list_size=${#installing[@]}

    printCompletedItems "${installed[@]}"
    if [ $installing_list_size -gt 0 ]
    then
        # echo "⌛ waiting for installing of: ${installing[@]}"
        sleep 3
    else
        echo "Kalm Installation Complete! 🎉"
        echo ""
        echo "To start using Kalm, open a port via:"
        echo ""
        echo "kubectl port-forward -n kalm-system \$(kubectl get pod -n kalm-system -l app=kalm -ojsonpath=\"{.items[0].metadata.name}\") 3010:3010"
        echo ""
        echo "Then visit http://localhost:3010 in your browser"
        echo ""
        finish="True"
    fi
done
