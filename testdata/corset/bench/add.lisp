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
  (stamp-constancy STAMP ARG_1_HI))

(defconstraint stamp-constancy-arg-1-lo ()
  (stamp-constancy STAMP ARG_1_LO))

(defconstraint stamp-constancy-arg-2-hi ()
  (stamp-constancy STAMP ARG_2_HI))

(defconstraint stamp-constancy-arg-2-lo ()
  (stamp-constancy STAMP ARG_2_LO))

(defconstraint stamp-constancy-res-hi ()
  (stamp-constancy STAMP RES_HI))

(defconstraint stamp-constancy-res-lo ()
  (stamp-constancy STAMP RES_LO))

(defconstraint stamp-constancy-inst ()
  (stamp-constancy STAMP INST))

(defconstraint stamp-constancy-ct-max ()
  (stamp-constancy STAMP CT_MAX))

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
  (∨ (will-remain-constant! STAMP) (will-inc! STAMP 1)))

;; When stamp increases, counter is reset
(defconstraint heartbeat-counter-reset ()
  (if (¬ (will-remain-constant! STAMP))
      (== (next CT) 0)))

;; outside of padding, instruction either ADD or SUB
(defconstraint heartbeat-instruction ()
  (if (!= STAMP 0)
      (∨ (eq! INST EVM_INST_ADD) (eq! INST EVM_INST_SUB))))

(defconstraint heartbeat-counter-increments ()
  (if (!= STAMP 0)
      (if (== CT CT_MAX)
          ;; After last row of frame, stamp increases
          (will-inc! STAMP 1)
          ;; On rows within frame, counter increases
          (will-inc! CT 1))))

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
  (byte-decomposition CT ACC_1 BYTE_1))

(defconstraint byte-decomposition-acc-2 ()
  (byte-decomposition CT ACC_2 BYTE_2))

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
                 (* THETA (prev OVERFLOW)))))))

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
                 (* THETA (prev OVERFLOW)))))))
