;; Conditional Module Test
(defconst
  LONDON_FORK 1
  SHANGHAI_FORK 2)

(defconst (EVM_FORK :i8) LONDON_FORK)

;; Module suitable for all forks
(module all)
(defcolumns (X :i16) (Y :i16))
(defconstraint c1 (:guard X) (!= X Y))

(deflookup l1 (Y) (lon.A))
(deflookup l2 (Y) (shan.U))

;; Module suitable only for london
(module lon (== EVM_FORK LONDON_FORK))
(defcolumns (A :i16) (B :i16))
(defconstraint c2 (:guard A) (!= A B))

;; Module suitable only for shanghai
(module shan (== EVM_FORK SHANGHAI_FORK))
(defcolumns (U :i16) (W :i16))
(defconstraint c3 (:guard U) (!= U W))
