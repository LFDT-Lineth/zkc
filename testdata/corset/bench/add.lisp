(module add)

(defcolumns
  (STAMP :i32)
  (CT_MAX :byte)
  (CT :byte)
  (INST :byte :display :opcode)
  (ARG_1_HI :i128)
  (ARG_1_LO :i128)
  (ARG_2_HI :i128)
  (ARG_2_LO :i128)
  (RES_HI :i128)
  (RES_LO :i128)
  (BYTE_1 :byte@prove)
  (BYTE_2 :byte@prove)
  (ACC_1 :i128)
  (ACC_2 :i128)
  (OVERFLOW :binary@prove))

(defconst
  EVM_INST_STOP                          0x00
  EVM_INST_ADD                           0x01
  EVM_INST_MUL                           0x02
  EVM_INST_SUB                           0x03
  LLARGEMO 15
  LLARGE   16
  THETA 340282366920938463463374607431768211456) ;; note that 340282366920938463463374607431768211456 = 256^16

(defconstraint stamp-constancy-arg-1-hi ()
  (if (== (shift STAMP 1) STAMP) (== (shift ARG_1_HI 1) ARG_1_HI)))

(defconstraint stamp-constancy-arg-1-lo ()
  (if (== (shift STAMP 1) STAMP) (== (shift ARG_1_LO 1) ARG_1_LO)))

(defconstraint stamp-constancy-arg-2-hi ()
  (if (== (shift STAMP 1) STAMP) (== (shift ARG_2_HI 1) ARG_2_HI)))

(defconstraint stamp-constancy-arg-2-lo ()
  (if (== (shift STAMP 1) STAMP) (== (shift ARG_2_LO 1) ARG_2_LO)))

(defconstraint stamp-constancy-res-hi ()
  (if (== (shift STAMP 1) STAMP) (== (shift RES_HI 1) RES_HI)))

(defconstraint stamp-constancy-res-lo ()
  (if (== (shift STAMP 1) STAMP) (== (shift RES_LO 1) RES_LO)))

(defconstraint stamp-constancy-inst ()
  (if (== (shift STAMP 1) STAMP) (== (shift INST 1) INST)))

(defconstraint stamp-constancy-ct-max ()
  (if (== (shift STAMP 1) STAMP) (== (shift CT_MAX 1) CT_MAX)))

;;;;;;;;;;;;;;;;;;;;;;;;;
;;                     ;;
;;    1.3 heartbeat    ;;
;;                     ;;
;;;;;;;;;;;;;;;;;;;;;;;;;
(defconstraint first-row (:domain {0})
  (== STAMP 0))

;; In padding rows, the instruction is zero
(defconstraint heartbeat-padding-inst ()
  (if (== STAMP 0)
      (== INST 0)))

;; Stamp either constant is increases by 1
(defconstraint heartbeat-stamp-increments ()
  (∨ (== (shift STAMP 1) STAMP) (== (shift STAMP 1) (+ STAMP 1))))

;; When stamp increases, counter is reset
(defconstraint heartbeat-counter-reset ()
  (if (¬ (== (shift STAMP 1) STAMP))
      (== (shift CT 1) 0)))

;; outside of padding, instruction either ADD or SUB
(defconstraint heartbeat-instruction ()
  (if (!= STAMP 0)
      (∨ (== INST EVM_INST_ADD) (== INST EVM_INST_SUB))))

(defconstraint heartbeat-counter-increments ()
  (if (!= STAMP 0)
      (if (== CT CT_MAX)
          ;; After last row of frame, stamp increases
          (== (shift STAMP 1) (+ STAMP 1))
          ;; On rows within frame, counter increases
          (== (shift CT 1) (+ CT 1)))))

;; (CT < LLARGE) ∧ (CT_MAX > 0)
(defconstraint heartbeat-counter-bounds ()
  (if (!= STAMP 0)
      (∧ (!= CT LLARGE) (!= CT_MAX 0))))

(defconstraint last-row (:domain {-1})
  (== CT CT_MAX))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;;                                                   ;;
;;    1.4 binary, bytehood and byte decompositions   ;;
;;                                                   ;;
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
(defconstraint byte-decomposition-acc-1 ()
  (if (== CT 0) (== ACC_1 BYTE_1) (== ACC_1 (+ (* 256 (shift ACC_1 -1)) BYTE_1))))

(defconstraint byte-decomposition-acc-2 ()
  (if (== CT 0) (== ACC_2 BYTE_2) (== ACC_2 (+ (* 256 (shift ACC_2 -1)) BYTE_2))))

;; TODO: bytehood constraints
;;;;;;;;;;;;;;;;;;;;;;;;;;;
;;                       ;;
;;    1.5 constraints    ;;
;;                       ;;
;;;;;;;;;;;;;;;;;;;;;;;;;;;
(defconstraint adder-result-hi (:guard STAMP)
  (if (== CT CT_MAX)
      (== RES_HI ACC_1)))

(defconstraint adder-result-lo (:guard STAMP)
  (if (== CT CT_MAX)
      (== RES_LO ACC_2)))

(defconstraint adder-addition-lo (:guard STAMP)
  (if (== CT CT_MAX)
      (if (!= INST EVM_INST_SUB)
          (== (+ ARG_1_LO ARG_2_LO)
              (+ RES_LO (* THETA OVERFLOW))))))

(defconstraint adder-addition-hi (:guard STAMP)
  (if (== CT CT_MAX)
      (if (!= INST EVM_INST_SUB)
          (== (+ ARG_1_HI ARG_2_HI OVERFLOW)
              (+ RES_HI
                 (* THETA (shift OVERFLOW -1)))))))

(defconstraint adder-subtraction-lo (:guard STAMP)
  (if (== CT CT_MAX)
      (if (!= INST EVM_INST_ADD)
          (== (+ RES_LO ARG_2_LO)
              (+ ARG_1_LO (* THETA OVERFLOW))))))

(defconstraint adder-subtraction-hi (:guard STAMP)
  (if (== CT CT_MAX)
      (if (!= INST EVM_INST_ADD)
          (== (+ RES_HI ARG_2_HI OVERFLOW)
              (+ ARG_1_HI
                 (* THETA (shift OVERFLOW -1)))))))
