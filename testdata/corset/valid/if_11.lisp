(defcolumns
  (CT :byte)
  (IS_SLT :binary@prove)
  (BITS :binary@prove)
  (NEG_2 :binary@prove))

(defconstraint bits-and-negs (:guard IS_SLT)
  (if (== CT 15)
    (== NEG_2 (shift BITS (- 0 7)))))
