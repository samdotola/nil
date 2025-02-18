import { COLORS } from "@nilfoundation/ui-kit";
import { MonoParagraphMedium } from "baseui/typography";
import { nanoid } from "nanoid";
import { compileCodeFx } from "../code/model";
import {
  assignSmartContractFx,
  callFx,
  deploySmartContractFx,
  registerContractInCometaFx,
  sendMethodFx,
} from "../contracts/models/base";
import { ContractDeployedLog } from "./components/ContractDeployedLog";
import { LogTitleWithDetails } from "./components/LogTitleWithDetails";
import { TransactionEventLog } from "./components/TransactionEventLog.tsx";
import { TransactionSentLog } from "./components/TransactionSentLog";
import { TxDetials } from "./components/TxDetails";
import { $logs, type Log, LogTopic, LogType, clearLogs } from "./model";
import { formatSolidityError } from "./utils";

$logs.on(deploySmartContractFx.doneData, (logs, { address, name, deployedFrom, txHash }) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Deployment,
      type: LogType.Success,
      shortDescription: (
        <LogTitleWithDetails
          title={
            <MonoParagraphMedium color={COLORS.green200}>
              {`Contract ${name} deployed from ${deployedFrom}`}
            </MonoParagraphMedium>
          }
          details={<TxDetials txHash={txHash} />}
        />
      ),
      payload: <ContractDeployedLog address={address} />,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(assignSmartContractFx.doneData, (logs, { assignedSmartContractAddress }) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Assign,
      type: LogType.Success,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.green200}>
          Contract assigned successfully
        </MonoParagraphMedium>
      ),
      payload: <ContractDeployedLog address={assignedSmartContractAddress} />,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(deploySmartContractFx.failData, (logs, error) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Deployment,
      type: LogType.Error,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.red200}>Deployment failed</MonoParagraphMedium>
      ),
      payload: <MonoParagraphMedium color={COLORS.red200}>{String(error)}</MonoParagraphMedium>,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(assignSmartContractFx.failData, (logs, error) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Deployment,
      type: LogType.Error,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.red200}>Assign failed</MonoParagraphMedium>
      ),
      payload: <MonoParagraphMedium color={COLORS.red200}>{String(error)}</MonoParagraphMedium>,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(compileCodeFx.doneData, (logs) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Compilation,
      type: LogType.Info,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.gray400}>Compilation successful</MonoParagraphMedium>
      ),
      timestamp: Date.now(),
    },
  ];
});

$logs.on(compileCodeFx.failData, (logs, error) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Compilation,
      type: LogType.Error,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.red200}>Compilation failed</MonoParagraphMedium>
      ),
      payload: (
        <MonoParagraphMedium color={COLORS.red200}>
          {formatSolidityError(String(error))}
        </MonoParagraphMedium>
      ),
      timestamp: Date.now(),
    },
  ];
});

$logs.on(callFx.doneData, (logs, { result, appName, functionName }) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Call,
      type: LogType.Success,
      shortDescription: (
        <MonoParagraphMedium
          color={COLORS.green200}
        >{`${appName}.${functionName}()`}</MonoParagraphMedium>
      ),
      payload: (
        <MonoParagraphMedium color={COLORS.gray400}>{`Result: ${result}`}</MonoParagraphMedium>
      ),
      timestamp: Date.now(),
    },
  ];
});

$logs.on(callFx.failData, (logs, error) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Call,
      type: LogType.Error,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.red200}>Call failed</MonoParagraphMedium>
      ),
      payload: <MonoParagraphMedium color={COLORS.red200}>{String(error)}</MonoParagraphMedium>,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(sendMethodFx.doneData, (logs, { hash, functionName, sendFrom, appName, txLogs }) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.SendTx,
      type: LogType.Success,
      shortDescription: (
        <LogTitleWithDetails
          title={
            <MonoParagraphMedium
              color={COLORS.green200}
            >{`${appName}.${functionName}() from ${sendFrom}`}</MonoParagraphMedium>
          }
          details={<TxDetials txHash={hash} />}
        />
      ),
      payload: <TransactionSentLog hash={hash} />,
      timestamp: Date.now(),
    },
    ...txLogs.map(
      (log): Log => ({
        id: nanoid(),
        topic: LogTopic.Log,
        type: LogType.Info,
        shortDescription: <TransactionEventLog message={log} />,
        timestamp: Date.now(),
      }),
    ),
  ];
});

$logs.on(sendMethodFx.failData, (logs, error) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.SendTx,
      type: LogType.Error,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.red200}>Transaction failed</MonoParagraphMedium>
      ),
      payload: <MonoParagraphMedium color={COLORS.red200}>{String(error)}</MonoParagraphMedium>,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(registerContractInCometaFx.failData, (logs, error) => {
  return [
    ...logs,
    {
      id: nanoid(),
      topic: LogTopic.Deployment,
      type: LogType.Warn,
      shortDescription: (
        <MonoParagraphMedium color={COLORS.yellow200}>
          Contract registration in Cometa failed. You won't be able to view the source code.
        </MonoParagraphMedium>
      ),
      payload: <MonoParagraphMedium color={COLORS.gray50}>{String(error)}</MonoParagraphMedium>,
      timestamp: Date.now(),
    },
  ];
});

$logs.on(clearLogs, () => []);
